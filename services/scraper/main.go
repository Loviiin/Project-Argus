package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"scraper/internal/worker"

	"github.com/loviiin/project-argus/pkg/config"
	"github.com/loviiin/project-argus/pkg/dedup"
	"github.com/loviiin/project-argus/pkg/healthcheck"
	"github.com/loviiin/project-argus/pkg/metrics"
	"github.com/loviiin/project-argus/pkg/natsutil"
	"github.com/loviiin/project-argus/pkg/tiktok"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	_ "modernc.org/sqlite"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg := config.LoadConfig()

	slog.Info("Argus Scraper Worker (Subscriber) iniciando (Arquitetura Sidecar)...")

	// Inicia Pprof em background
	go func() {
		slog.Info("Iniciando Pprof do Scraper", "porta", ":6060")
		if err := http.ListenAndServe("127.0.0.1:6060", nil); err != nil {
			slog.Error("Pprof server falhou", "error", err)
		}
	}()

	// --- NATS ---
	nc, err := nats.Connect(cfg.Nats.URL)
	if err != nil {
		slog.Error("Erro NATS", "error", err)
		os.Exit(1)
	}
	js, err := nc.JetStream()
	if err != nil {
		slog.Error("Erro JetStream", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	streamCfg := &nats.StreamConfig{
		Name:     "argus-scraper",
		Subjects: []string{"jobs.scrape", "jobs.scrape.>"},
		Storage:  nats.FileStorage,
	}
	_, err = js.AddStream(streamCfg)
	if err != nil && err != nats.ErrStreamNameAlreadyInUse {
		slog.Warn("Aviso ao configurar Stream SCRAPE", "error", err)
	}

	// Cria Stream DLQ do scraper para evitar loop infinito de NumDelivered > 15
	dlqCfg := &nats.StreamConfig{
		Name:     "argus-scraper-dlq",
		Subjects: []string{"argus.dlq.scraper"},
		Storage:  nats.FileStorage,
	}
	_, err = js.AddStream(dlqCfg)
	if err != nil && err != nats.ErrStreamNameAlreadyInUse {
		slog.Warn("Aviso ao configurar Stream DLQ", "error", err)
	}

	// --- Redis ---
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	dedupSv := dedup.NewDeduplicator(rdb, cfg.Redis.TTLHours)
	defer dedupSv.Close()

	// --- Database ---
	var db *sql.DB
	var dbErr error
	if cfg.Database.Type == "sqlite" {
		dbPath := cfg.Database.SQLitePath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=synchronous(NORMAL)"
		db, dbErr = sql.Open("sqlite", dbPath)
	} else {
		db, dbErr = sql.Open("postgres", cfg.Database.URL)
	}
	if dbErr != nil {
		slog.Error("Erro conexão Banco", "error", dbErr)
		os.Exit(1)
	}

	// Removido SetMaxOpenConns(1) para evitar starvation
	if err = db.Ping(); err != nil {
		slog.Error("Erro ping Banco", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// --- TikTok Signer Client ---
	signerClient := tiktok.NewSignerClient(cfg.TikTok.SidecarURL)

	// --- Worker Setup ---
	proc := worker.NewProcessor(cfg, signerClient, db, js)
	defer proc.Close()

	workerIDStr := os.Getenv("WORKER_ID")
	if workerIDStr == "" {
		workerIDStr = "1"
	}

	workerType := os.Getenv("WORKER_TYPE")
	if workerType == "" {
		workerType = "top_level"
	}

	subject := "jobs.scrape"
	group := "scraper-worker-group"
	numWorkers := cfg.Scraper.Workers

	if workerType == "reply" {
		subject = "jobs.scrape.reply"
		group = "scraper-reply-group"
		numWorkers = cfg.Scraper.ReplyWorkers
	}

	if numWorkers <= 0 {
		numWorkers = 1
	}

	scraperMetrics := []metrics.MetricDef{
		{RedisKey: "argus:metrics:scraper:scraped", PromName: "argus_scraper_scraped_total", Help: "Total de vídeos scraped", Type: "counter"},
		{RedisKey: "argus:metrics:scraper:comments", PromName: "argus_scraper_comments_total", Help: "Total de comentários extraídos", Type: "counter"},
		{RedisKey: "argus:metrics:scraper:errors", PromName: "argus_scraper_errors_total", Help: "Total de erros", Type: "counter"},
	}
	healthHandler := healthcheck.New(nc, rdb, nil).Handler
	go metrics.StartMetricsServer(":8084", rdb, scraperMetrics, healthHandler)

	// --- Subscriber ---
	sub, err := js.PullSubscribe(subject, group, nats.AckWait(10*time.Minute))
	if err != nil {
		slog.Error("Erro ao criar pull subscriber", "error", err)
		os.Exit(1)
	}
	defer sub.Unsubscribe()

	slog.Info("Scraper Worker rodando!", "worker_type", workerType, "worker_id", workerIDStr, "subject", subject, "max_workers", numWorkers)

	// Aguarda sinal de parada
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		slog.Info("Sinal recebido. Encerrando Scraper Worker (Aguardando rotinas atuais)...", "worker_type", workerType)
		cancel()
	}()

	sem := make(chan struct{}, numWorkers) // Max goroutines para chamadas de API
	var wg sync.WaitGroup

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		default:
		}

		msgs, err := sub.Fetch(1, nats.MaxWait(5*time.Second))
		if err != nil {
			if err == nats.ErrTimeout || err == nats.ErrConnectionClosed || err == nats.ErrBadSubscription {
				continue // Nenhuma mensagem na fila ou dreno iniciando
			}
			slog.Error("Erro no Fetch", "worker_id", workerIDStr, "error", err)
			time.Sleep(2 * time.Second)
			continue
		}

		msg := msgs[0]

		sem <- struct{}{}
		wg.Add(1)

		handler := natsutil.SafeHandler(func(m *nats.Msg) {
			meta, err := m.Metadata()
			if err != nil {
				slog.Error("Erro lendo metadata", "worker_id", workerIDStr, "error", err)
				m.Ack()
				return
			}

			var job worker.ScrapeJob
			if err := json.Unmarshal(m.Data, &job); err != nil {
				slog.Error("Erro unmarshal job", "worker_id", workerIDStr, "error", err)
				m.Ack() // Ack porque falha de parse não resolve com retry
				return
			}

			slog.Info("Recebido job", "worker_id", workerIDStr, "video_id", job.VideoID, "cursor", job.Cursor, "tentativa", meta.NumDelivered)

			// 1. Worker Heartbeat/Processing Lock
			lockSuffix := fmt.Sprintf("%s:%d", job.VideoID, job.Cursor)
			if job.CommentID != "" {
				lockSuffix = fmt.Sprintf("%s:%s:%d", job.VideoID, job.CommentID, job.Cursor)
			}
			lockKey := fmt.Sprintf("argus:processing_lock:%s", lockSuffix)

			if locked, _ := dedupSv.RDB().SetNX(ctx, lockKey, "1", 10*time.Minute).Result(); !locked {
				delay := time.Duration(30+rand.Intn(30)) * time.Second
				slog.Warn("Job bloqueado por lock. Nak + Jitter", "worker_id", workerIDStr, "job", lockSuffix, "delay", delay)
				m.NakWithDelay(delay)
				return
			}
			defer dedupSv.RDB().Del(ctx, lockKey)

			// 2. Dead Letter Queue (DLQ)
			if meta.NumDelivered > 15 {
				slog.Warn("Max Retries atingido. Enviando para DLQ...", "worker_id", workerIDStr, "video_id", job.VideoID)
				dlqPayload := map[string]interface{}{
					"error": "Max retries exceeded",
					"job":   job,
					"metadata": map[string]interface{}{
						"num_delivered": meta.NumDelivered,
						"timestamp":     time.Now(),
					},
				}
				dlqData, _ := json.Marshal(dlqPayload)
				if _, err := js.Publish("argus.dlq.scraper", dlqData); err != nil {
					slog.Error("Erro publicando DLQ", "worker_id", workerIDStr, "error", err)
					m.NakWithDelay(1 * time.Minute)
					return
				}
				m.Ack()
				return
			}

			// Processa o vídeo via Sidecar HTTP
			insertedCount, err := proc.ProcessVideo(ctx, job)
			if err != nil {
				slog.Error("Erro processando job", "worker_id", workerIDStr, "video_id", job.VideoID, "error", err)
				rdb.Incr(ctx, "argus:metrics:scraper:errors")
				// Exponential Backoff Nak
				delay := time.Duration(10+rand.Intn(20)) * time.Second
				slog.Info("Nak no job com delay", "worker_id", workerIDStr, "video_id", job.VideoID, "delay", delay)
				m.NakWithDelay(delay)
				return
			}

			rdb.Incr(ctx, "argus:metrics:scraper:scraped")
			if insertedCount > 0 {
				rdb.IncrBy(ctx, "argus:metrics:scraper:comments", int64(insertedCount))
			}

			// Ack → confirma processamento bem-sucedido e dados inseridos no PG
			m.Ack()

			worker.RandomDelay(3, 8)
		})

		go func(m *nats.Msg) {
			defer wg.Done()
			defer func() { <-sem }()
			handler(m)
		}(msg)
	}

	slog.Info("Aguardando término das rotinas ativas...")
	wg.Wait()
	slog.Info("Scraper Worker encerrado gracefully.")
}
