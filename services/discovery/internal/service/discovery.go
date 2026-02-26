package service

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"discovery/internal/sources"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

// ScrapeJob é o payload publicado no tópico jobs.scrape.
// Contém apenas o mínimo necessário para o Worker navegar até o vídeo.
type ScrapeJob struct {
	VideoID  string `json:"video_id"`
	VideoURL string `json:"video_url"`
	Hashtag  string `json:"hashtag"`
}

type DiscoveryService struct {
	js          nats.JetStreamContext
	rdb         *redis.Client
	sources     []sources.Source
	concurrency int
}

func NewDiscoveryService(js nats.JetStreamContext, rdb *redis.Client, srcs []sources.Source, workers int) *DiscoveryService {
	if workers <= 0 {
		workers = 1
	}
	return &DiscoveryService{
		js:          js,
		rdb:         rdb,
		sources:     srcs,
		concurrency: workers,
	}
}

// EnsureStream garante que o stream SCRAPE exista no JetStream.
func EnsureStream(js nats.JetStreamContext) error {
	_, err := js.AddStream(&nats.StreamConfig{
		Name:     "SCRAPE",
		Subjects: []string{"jobs.scrape"},
		Storage:  nats.FileStorage,
	})
	if err != nil {
		// Se o stream já existe, não é erro
		if err == nats.ErrStreamNameAlreadyInUse {
			return nil
		}
		// Tenta atualizar se já existe com config diferente
		_, updateErr := js.UpdateStream(&nats.StreamConfig{
			Name:     "SCRAPE",
			Subjects: []string{"jobs.scrape"},
			Storage:  nats.FileStorage,
		})
		if updateErr != nil {
			return err
		}
	}
	return nil
}

// Close encerra a conexão com todos os sources (ex: fecha navegadores)
func (s *DiscoveryService) Close() {
	for _, src := range s.sources {
		if err := src.Close(); err != nil {
			log.Printf("Erro ao fechar source %s: %v", src.Name(), err)
		}
	}
}

// Run executa um ciclo de discovery: descobre URLs novas e publica no NATS.
func (s *DiscoveryService) Run(hashtags []string) {
	ctx := context.Background()

	for _, src := range s.sources {
		sem := make(chan struct{}, s.concurrency)
		var wg sync.WaitGroup

		for _, tag := range hashtags {
			wg.Add(1)
			sem <- struct{}{}

			go func(tag string) {
				defer wg.Done()
				defer func() { <-sem }()

				log.Printf("[%s] buscando via %s", tag, src.Name())
				videos, err := src.Fetch(ctx, tag)
				if err != nil {
					log.Printf("[%s] erro: %v", tag, err)
					return
				}
				log.Printf("[%s] %d vídeos novos descobertos", tag, len(videos))

				// Publica cada vídeo novo no tópico jobs.scrape
				for _, v := range videos {
					job := ScrapeJob{
						VideoID:  v.ID,
						VideoURL: v.URL,
						Hashtag:  tag,
					}

					data, err := json.Marshal(job)
					if err != nil {
						log.Printf("[%s] erro marshal job %s: %v", tag, v.ID, err)
						continue
					}

					_, err = s.js.Publish("jobs.scrape", data)
					if err != nil {
						log.Printf("[%s] erro publicar job %s: %v", tag, v.ID, err)
						s.rdb.Incr(ctx, "argus:metrics:discovery:failed")
					} else {
						log.Printf("[%s] ✅ job publicado: %s → jobs.scrape", tag, v.ID)
						s.rdb.Incr(ctx, "argus:metrics:discovery:enqueued")
					}
				}
			}(tag)
		}

		wg.Wait()
	}
}
