package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"scraper/internal/worker"

	_ "github.com/lib/pq"
	"github.com/loviiin/project-argus/pkg/config"
	"github.com/loviiin/project-argus/pkg/dedup"
	"github.com/loviiin/project-argus/pkg/session"
	"github.com/loviiin/project-argus/pkg/tiktok"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.LoadConfig()

	fmt.Println("Argus Scraper Worker (Subscriber) iniciando (Arquitetura Sidecar)...")

	// --- NATS ---
	nc, err := nats.Connect(cfg.Nats.URL)
	if err != nil {
		log.Fatal("Erro NATS:", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		log.Fatal("Erro JetStream:", err)
	}
	defer nc.Close()

	// Garante que o stream SCRAPE exista
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "SCRAPE",
		Subjects: []string{"jobs.scrape"},
		Storage:  nats.FileStorage,
	})
	if err != nil {
		log.Printf("Stream SCRAPE: %v (ok se já existe)", err)
	}

	// --- Redis ---
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	dedupSv := dedup.NewDeduplicator(rdb, cfg.Redis.TTLHours)
	defer dedupSv.Close()

	sessionManager := session.NewManager(rdb)

	// --- PostgreSQL ---
	db, err := sql.Open("postgres", cfg.Database.URL)
	if err != nil {
		log.Fatal("Erro conexão PostgreSQL:", err)
	}
	if err = db.Ping(); err != nil {
		log.Fatal("Erro ping PostgreSQL:", err)
	}
	defer db.Close()

	// --- TikTok Signer Client ---
	signerClient := tiktok.NewSignerClient(cfg.TikTok.SidecarURL)

	// --- Worker Setup ---
	proc := worker.NewProcessor(cfg, sessionManager, signerClient, db, js)
	defer proc.Close()

	workerIDStr := os.Getenv("WORKER_ID")
	if workerIDStr == "" {
		workerIDStr = "1"
	}

	// --- Subscriber ---
	sub, err := js.PullSubscribe("jobs.scrape", "scraper-worker-group", nats.AckWait(10*time.Minute))
	if err != nil {
		log.Fatal("Erro ao criar pull subscriber:", err)
	}
	defer sub.Unsubscribe()

	log.Printf("Scraper Worker %s rodando! Consumindo jobs.scrape...", workerIDStr)

	// Aguarda sinal de parada
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\nSinal recebido. Encerrando Scraper Worker (Aguardando rotinas atuais)...")
		cancel()
	}()

	numWorkers := cfg.Scraper.Workers
	if numWorkers <= 0 {
		numWorkers = 1
	}
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
			log.Printf("[Worker %s] Erro no Fetch: %v", workerIDStr, err)
			time.Sleep(2 * time.Second)
			continue
		}

		msg := msgs[0]

		sem <- struct{}{}
		wg.Add(1)

		go func(m *nats.Msg) {
			defer wg.Done()
			defer func() { <-sem }()

			meta, err := m.Metadata()
			if err != nil {
				log.Printf("[Worker %s] ❌ Erro lendo metadata: %v", workerIDStr, err)
				m.Ack()
				return
			}

			var job worker.ScrapeJob
			if err := json.Unmarshal(m.Data, &job); err != nil {
				log.Printf("[Worker %s] ❌ erro unmarshal job: %v", workerIDStr, err)
				m.Ack() // Ack porque falha de parse não resolve com retry
				return
			}

			log.Printf("[Worker %s] 📥 Recebido job: %s (Cursor: %d) [Tentativa: %d]", workerIDStr, job.VideoID, job.Cursor, meta.NumDelivered)

			// 1. Worker Heartbeat/Processing Lock
			lockKey := fmt.Sprintf("argus:processing_lock:%s:%d", job.VideoID, job.Cursor)
			if locked, _ := dedupSv.RDB().SetNX(ctx, lockKey, "1", 10*time.Minute).Result(); !locked {
				delay := time.Duration(30+rand.Intn(30)) * time.Second
				log.Printf("[Worker %s] Job %s (Cursor: %d) bloqueado por lock. Nak + Jitter: %v", workerIDStr, job.VideoID, job.Cursor, delay)
				m.NakWithDelay(delay)
				return
			}
			defer dedupSv.RDB().Del(ctx, lockKey)

			// 2. Dead Letter Queue (DLQ)
			if meta.NumDelivered > 15 {
				log.Printf("[Worker %s] 🚨 Max Retries atingido para %s. Enviando para DLQ...", workerIDStr, job.VideoID)
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
					log.Printf("[Worker %s] ❌ erro publicando DLQ: %v", workerIDStr, err)
					m.NakWithDelay(1 * time.Minute)
					return
				}
				m.Ack()
				return
			}

			// Processa o vídeo via Sidecar HTTP
			err = proc.ProcessVideo(ctx, job)
			if err != nil {
				log.Printf("[Worker %s] ❌ erro processando %s: %v", workerIDStr, job.VideoID, err)
				// Exponential Backoff Nak
				delay := time.Duration(10+rand.Intn(20)) * time.Second
				log.Printf("[Worker %s] ⏳ Nak no job %s com delay de %v", workerIDStr, job.VideoID, delay)
				m.NakWithDelay(delay)
				return
			}

			// Ack → confirma processamento bem-sucedido e dados inseridos no PG
			m.Ack()

			// Delay anti-rate-limit entre jobs (3-8 segundos) para não estressar logo após
			worker.RandomDelay(3, 8)
		}(msg)
	}

	fmt.Println("[Worker] Aguardando término das rotinas ativas...")
	wg.Wait()
	fmt.Println("[Worker] Scraper Worker encerrado gracefully.")
}
