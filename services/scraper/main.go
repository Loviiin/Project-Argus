package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"scraper/internal/worker"

	"github.com/loviiin/project-argus/pkg/config"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

// Deduplicator é uma cópia mínima para o worker marcar vídeos como vistos.
type Deduplicator struct {
	rdb *redis.Client
}

func NewDeduplicator(address, password string, db int) *Deduplicator {
	rdb := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       db,
	})
	return &Deduplicator{rdb: rdb}
}

func (d *Deduplicator) MarkAsSeen(ctx context.Context, videoID string) error {
	key := fmt.Sprintf("argus:seen:%s", videoID)
	_, err := d.rdb.Set(ctx, key, "1", 7*24*60*60*1e9).Result() // 7 dias TTL
	return err
}

func (d *Deduplicator) Close() error {
	return d.rdb.Close()
}

func main() {
	cfg := config.LoadConfig()

	fmt.Println("Argus Scraper Worker (Subscriber) iniciando...")

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
	dedup := NewDeduplicator(cfg.Redis.Address, cfg.Redis.Password, cfg.Redis.DB)
	defer dedup.Close()

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
	// Para balancear a carga entre os workers, todos devem usar o mesmo nome "durable"
	// Extendemos o AckWait para 10 minutos para evitar redelivery no meio do scraping de vídeos muito longos
	sub, err := js.PullSubscribe("jobs.scrape", "scraper-worker-group", nats.AckWait(10*time.Minute))
	if err != nil {
		log.Fatal("Erro ao criar pull subscriber:", err)
	}
	defer sub.Unsubscribe()

	log.Printf("Scraper Worker %s rodando! Consumindo jobs.scrape sequencialmente...", workerIDStr)

	// Aguarda sinal de parada
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\nSinal recebido. Encerrando Scraper Worker...")
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := sub.Fetch(1, nats.MaxWait(10*time.Second))
		if err != nil {
			if err == nats.ErrTimeout {
				continue // Nenhuma mensagem na fila
			}
			log.Printf("[Worker %s] Erro no Fetch: %v", workerIDStr, err)
			time.Sleep(2 * time.Second)
			continue
		}

		msg := msgs[0]
		var job worker.ScrapeJob
		if err := json.Unmarshal(msg.Data, &job); err != nil {
			log.Printf("[Worker %s] ❌ erro unmarshal job: %v", workerIDStr, err)
			msg.Nak()
			continue
		}

		log.Printf("[Worker %s] 📥 Recebido job: %s (%s)", workerIDStr, job.VideoID, job.Hashtag)

		// Processa o vídeo
		payload, err := worker.ProcessVideo(browser, job)
		if err != nil {
			log.Printf("[Worker %s] ❌ erro processando %s: %v", workerIDStr, job.VideoID, err)
			// Devolve para a fila em caso de erro no processamento
			msg.Nak()
			continue
		}

		// Se não capturou nenhum comentário, skip e segue para o próximo
		if payload.Metadata != nil {
			if comments, ok := payload.Metadata["comments"]; ok {
				if arr, ok := comments.([]interface{}); ok && len(arr) == 0 {
					log.Printf("[Worker %s] ⏩ skip (0 comentários): %s", workerIDStr, job.VideoID)
					msg.Ack()
					continue
				}
			}
		}

		// Publica o resultado no tópico data.text_extracted
		data, err := json.Marshal(payload)
		if err != nil {
			log.Printf("[Worker %s] ❌ erro marshal payload %s: %v", workerIDStr, job.VideoID, err)
			msg.Nak()
			continue
		}

		_, err = js.Publish("data.text_extracted", data)
		if err != nil {
			log.Printf("[Worker %s] ❌ erro publicar resultado %s: %v", workerIDStr, job.VideoID, err)
			// Devolve para a fila usando Nak
			msg.Nak()
			continue
		}

		log.Printf("[Worker %s] ✅ Publicado: %s → data.text_extracted", workerIDStr, job.VideoID)

		// Só marca como visto DEPOIS do publish com sucesso
		if err := dedup.MarkAsSeen(ctx, job.VideoID); err != nil {
			log.Printf("[Worker %s] ⚠️  erro redis MarkAsSeen %s: %v", workerIDStr, job.VideoID, err)
		}

		// Ack → confirma processamento bem-sucedido
		msg.Ack()

		// Delay anti-rate-limit entre jobs (3-8 segundos)
		worker.RandomDelay(3, 8)
	}
}
