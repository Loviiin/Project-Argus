package main

import (
	"context"
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

	"github.com/loviiin/project-argus/pkg/config"
	"github.com/loviiin/project-argus/pkg/dedup"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.LoadConfig()

	fmt.Println("Argus Scraper Worker (Subscriber) iniciando...")

	// Inicia rotina do Garbage Collector de perfis no background
	go worker.StartProfileSweeper()

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

	// Garante que o stream DATA exista para data.text_extracted
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "DATA",
		Subjects: []string{"data.text_extracted"},
		Storage:  nats.FileStorage,
	})
	if err != nil {
		log.Printf("Stream DATA: %v (ok se já existe)", err)
	}

	// --- Redis ---
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	dedupSv := dedup.NewDeduplicator(rdb, cfg.Redis.TTLHours)
	defer dedupSv.Close()

	// --- Worker Setup ---
	workerIDStr := os.Getenv("WORKER_ID")
	if workerIDStr == "" {
		workerIDStr = "1"
	}
	workerIDInt := 1
	fmt.Sscanf(workerIDStr, "%d", &workerIDInt)

	// --- Browser ---
	browserStateDir := fmt.Sprintf("./browser_state_worker_%s", workerIDStr)
	debugPort := fmt.Sprintf(":%d", 9222+workerIDInt)

	browser, err := worker.NewBrowser(browserStateDir, debugPort)
	if err != nil {
		log.Fatal("Erro ao iniciar browser:", err)
	}
	defer browser.Close()

	log.Printf("Browser iniciado com estado em: %s", browserStateDir)
	log.Printf("⚠️  Se captcha aparecer, resolva via VNC (monitor em %s)", debugPort)

	// --- Subscriber ---
	// Extendemos o AckWait para 10 minutos para evitar redelivery no meio do scraping de vídeos muito longos
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

	sem := make(chan struct{}, 1) // Max 1 browser simultâneo por worker
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

			log.Printf("[Worker %s] 📥 Recebido job: %s (%s) [Tentativa: %d]", workerIDStr, job.VideoID, job.Hashtag, meta.NumDelivered)

			// 1. Worker Heartbeat/Processing Lock
			lockKey := fmt.Sprintf("argus:processing_lock:%s", job.VideoID)
			if locked, _ := dedupSv.RDB().SetNX(ctx, lockKey, "1", 10*time.Minute).Result(); !locked {
				delay := time.Duration(30+rand.Intn(30)) * time.Second
				log.Printf("[Worker %s] Job %s bloqueado por lock. Nak + Jitter: %v", workerIDStr, job.VideoID, delay)
				m.NakWithDelay(delay)
				return
			}
			defer dedupSv.RDB().Del(ctx, lockKey)

			// 2. Padrão de Idempotência Definitiva
			processed, err := dedupSv.CheckIfProcessed(ctx, "processed_job", job.VideoID)
			if err == nil && processed {
				log.Printf("[Worker %s] Mensagem duplicada ignorada: %s", workerIDStr, job.VideoID)
				m.Ack()
				return
			}

			// 3. Dead Letter Queue (DLQ)
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

			// Processa o vídeo
			payload, err := worker.ProcessVideo(browser, job)
			if err != nil {
				log.Printf("[Worker %s] ❌ erro processando %s: %v", workerIDStr, job.VideoID, err)
				// 2. Exponential Backoff Nak
				delay := time.Duration(10+rand.Intn(20)) * time.Second // Jittered delay
				log.Printf("[Worker %s] ⏳ Nak no job %s com delay de %v", workerIDStr, job.VideoID, delay)
				m.NakWithDelay(delay)
				return
			}

			// Se não capturou nenhum comentário, skip e segue para o próximo
			if payload.Metadata != nil {
				if comments, ok := payload.Metadata["comments"]; ok {
					if arr, ok := comments.([]interface{}); ok && len(arr) == 0 {
						log.Printf("[Worker %s] ⏩ skip (0 comentários): %s", workerIDStr, job.VideoID)
						m.Ack()
						return
					}
				}
			}

			// Publica o resultado no tópico data.text_extracted
			data, err := json.Marshal(payload)
			if err != nil {
				log.Printf("[Worker %s] ❌ erro marshal payload %s: %v", workerIDStr, job.VideoID, err)
				delay := time.Duration(10+rand.Intn(20)) * time.Second // Jittered delay
				m.NakWithDelay(delay)
				return
			}

			_, err = js.Publish("data.text_extracted", data)
			if err != nil {
				log.Printf("[Worker %s] ❌ erro publicar resultado %s: %v", workerIDStr, job.VideoID, err)
				delay := time.Duration(10+rand.Intn(20)) * time.Second // Jittered delay
				m.NakWithDelay(delay)
				return
			}

			log.Printf("[Worker %s] ✅ Publicado: %s → data.text_extracted", workerIDStr, job.VideoID)

			// Só marca como visto DEPOIS do publish com sucesso (Idempotência final)
			if err := dedupSv.MarkAsSeen(ctx, "processed_job", job.VideoID); err != nil {
				log.Printf("[Worker %s] ⚠️  erro redis MarkAsSeen %s: %v", workerIDStr, job.VideoID, err)
			}

			// Ack → confirma processamento bem-sucedido
			m.Ack()

			// Delay anti-rate-limit entre jobs (3-8 segundos) para não estressar logo após
			worker.RandomDelay(3, 8)
		}(msg)
	}

	fmt.Println("[Worker] Aguardando término das rotinas ativas...")
	wg.Wait()
	fmt.Println("[Worker] Scraper Worker encerrado gracefully.")
}
