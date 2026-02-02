package service

import (
	"context"
	"discovery/internal/repository"
	"discovery/internal/sources"
	"fmt"
	"log"

	"github.com/nats-io/nats.go"
)

type DiscoveryService struct {
	dedup   *repository.Deduplicator
	js      nats.JetStreamContext
	sources []sources.Source
}

func NewDiscoveryService(dedup *repository.Deduplicator, js nats.JetStreamContext) *DiscoveryService {
	return &DiscoveryService{
		dedup: dedup,
		js:    js,
		sources: []sources.Source{
			&sources.MockSource{},
		},
	}
}

func (s *DiscoveryService) Run(hashtags []string) {
	ctx := context.Background()

	for _, src := range s.sources {
		for _, tag := range hashtags {
			videos, err := src.FetchRecent(tag)
			if err != nil {
				log.Printf("Erro na fonte %s: %v", src.Name(), err)
				continue
			}

			for _, v := range videos {
				isNew, err := s.dedup.IsNew(ctx, v.URL)
				if err != nil {
					log.Printf("Erro verificando duplicidade: %v", err)
					continue
				}

				if !isNew {
					fmt.Printf("[Cache HIT] Vídeo duplicado ignorado: %s\n", v.URL)
					continue
				}

				fmt.Printf("[Cache MISS] NOVO ALVO ENCONTRADO: %s\n", v.URL)

				_, err = s.js.Publish("jobs.scrape", []byte(v.URL))
				if err != nil {
					log.Printf("Erro ao publicar job: %v", err)
				}
			}
		}
	}
}
