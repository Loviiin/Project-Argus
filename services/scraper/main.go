package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

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

	// --- Browser ---
	browserStateDir := "./browser_state_worker"
	if cfg.Scraper.BrowserStateDir != "" {
		browserStateDir = cfg.Scraper.BrowserStateDir
	}

	browser, err := worker.NewBrowser(browserStateDir)
	if err != nil {
		log.Fatal("Erro ao iniciar browser:", err)
	}
	defer browser.Close()

	log.Printf("Browser iniciado com estado em: %s", browserStateDir)
	log.Println("⚠️  Se captcha aparecer, resolva via VNC (monitor em :9223)")

	// --- Subscriber ---
	maxWorkers := cfg.Scraper.Workers
	if maxWorkers <= 0 {
		maxWorkers = 1
	}

	sub, err := js.Subscribe("jobs.scrape", func(msg *nats.Msg) {
		var job worker.ScrapeJob
		if err := json.Unmarshal(msg.Data, &job); err != nil {
			log.Printf("[Worker] ❌ erro unmarshal job: %v", err)
			msg.Nak()
			return
		}

		log.Printf("[Worker] 📥 Recebido job: %s (%s)", job.VideoID, job.Hashtag)

		// Processa o vídeo
		payload, err := worker.ProcessVideo(browser, job)
		if err != nil {
			log.Printf("[Worker] ❌ erro processando %s: %v", job.VideoID, err)
			msg.Nak()
			return
		}

		// Se não capturou nenhum comentário, skip e segue para o próximo
		if payload.Metadata != nil {
			if comments, ok := payload.Metadata["comments"]; ok {
				if arr, ok := comments.([]interface{}); ok && len(arr) == 0 {
					log.Printf("[Worker] ⏩ skip (0 comentários): %s", job.VideoID)
					msg.Ack()
					return
				}
			}
		}

		// Publica o resultado no tópico data.text_extracted
		data, err := json.Marshal(payload)
		if err != nil {
			log.Printf("[Worker] ❌ erro marshal payload %s: %v", job.VideoID, err)
			msg.Nak()
			return
		}

		_, err = js.Publish("data.text_extracted", data)
		if err != nil {
			log.Printf("[Worker] ❌ erro publicar resultado %s: %v", job.VideoID, err)
			// Nak → sem MarkAsSeen, mensagem volta para retry
			msg.Nak()
			return
		}

		log.Printf("[Worker] ✅ Publicado: %s → data.text_extracted", job.VideoID)

		// Só marca como visto DEPOIS do publish com sucesso
		ctx := context.Background()
		if err := dedup.MarkAsSeen(ctx, job.VideoID); err != nil {
			log.Printf("[Worker] ⚠️  erro redis MarkAsSeen %s: %v", job.VideoID, err)
			// Ainda faz Ack porque o dado já foi publicado com sucesso
		}

		// Ack → confirma processamento bem-sucedido
		msg.Ack()

		// Delay anti-rate-limit entre jobs (3-8 segundos)
		worker.RandomDelay(3, 8)

	}, nats.Durable("scraper-worker"),
		nats.ManualAck(),
		nats.MaxAckPending(maxWorkers),
		nats.AckWait(5*60*1e9), // 5 minutos para processar
	)
	if err != nil {
		log.Fatal("Erro ao criar subscriber:", err)
	}
	defer sub.Unsubscribe()

	log.Printf("Scraper Worker rodando! Consumindo jobs.scrape (max %d simultâneos)...", maxWorkers)

	// Aguarda sinal de parada
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	fmt.Println("\nEncerrando Scraper Worker...")
}
