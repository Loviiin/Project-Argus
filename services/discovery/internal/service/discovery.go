package service

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"time"

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
	Desc     string `json:"desc"`
	Author   string `json:"author"`
}

type DiscoveryService struct {
	js          nats.JetStreamContext
	rdb         *redis.Client
	sources     []sources.Source
	concurrency int
}

const (
	// Evita rajadas no endpoint de busca hashtag do TikTok.
	stage1DelayMin = 8 * time.Second
	stage1DelayMax = 16 * time.Second
)

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
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for _, src := range s.sources {
		if src.Name() == "TikTok-User-Tracker" {
			trackerTag := "@tracked_accounts"
			log.Printf("[%s] buscando via %s (1 execução por ciclo)", trackerTag, src.Name())
			videos, err := src.Fetch(ctx, "")
			if err != nil {
				log.Printf("[%s] erro: %v", trackerTag, err)
				continue
			}
			log.Printf("[%s] %d vídeos novos descobertos", trackerTag, len(videos))
			s.publishJobs(ctx, trackerTag, videos)
			continue
		}

		for i, tag := range hashtags {
			if i > 0 {
				delay := randomDuration(rng, stage1DelayMin, stage1DelayMax)
				log.Printf("[%s] aguardando %s antes da próxima busca hashtag", tag, delay)
				time.Sleep(delay)
			}

			log.Printf("[%s] buscando via %s", tag, src.Name())
			videos, err := src.Fetch(ctx, tag)
			if err != nil {
				log.Printf("[%s] erro: %v", tag, err)
				continue
			}
			log.Printf("[%s] %d vídeos novos descobertos", tag, len(videos))
			s.publishJobs(ctx, tag, videos)
		}
	}
}

func (s *DiscoveryService) publishJobs(ctx context.Context, tag string, videos []sources.DiscoveredVideo) {
	for _, v := range videos {
		job := ScrapeJob{
			VideoID:  v.ID,
			VideoURL: v.URL,
			Hashtag:  tag,
			Desc:     v.Desc,
			Author:   v.Author,
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
}

func randomDuration(rng *rand.Rand, min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	span := max - min
	return min + time.Duration(rng.Int63n(int64(span)+1))
}
