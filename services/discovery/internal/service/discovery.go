package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"discovery/internal/repository"
	"discovery/internal/sources"

	"github.com/nats-io/nats.go"
)

type DiscoveryService struct {
	dedup   *repository.Deduplicator
	js      nats.JetStreamContext
	sources []sources.Source
}

func NewDiscoveryService(dedup *repository.Deduplicator, js nats.JetStreamContext, srcs []sources.Source) *DiscoveryService {
	return &DiscoveryService{
		dedup:   dedup,
		js:      js,
		sources: srcs,
	}
}

type ArtifactPayload struct {
	SourcePath  string                 `json:"source_path"`
	TextContent string                 `json:"text_content"`
	AuthorID    string                 `json:"author_id,omitempty"`
	SourceType  string                 `json:"source_type,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

func (s *DiscoveryService) Run(hashtags []string) {
	ctx := context.Background()

	for _, src := range s.sources {
		for _, tag := range hashtags {
			log.Printf("Buscando '%s' via %s...", tag, src.Name())

			videos, err := src.Fetch(tag)
			if err != nil {
				log.Printf("Erro ao buscar na fonte %s: %v", src.Name(), err)
				continue
			}

			log.Printf("Encontrados %d vídeos. Processando...", len(videos))

			for _, v := range videos {
				isNew, err := s.dedup.IsNew(ctx, v.ID)
				if err != nil {
					log.Printf("Erro no Redis: %v", err)
				}
				if err == nil && !isNew {
					continue
				}

				fullText := v.Description + "\n" + fmt.Sprintf("%v", v.Comments)

				payload := ArtifactPayload{
					SourcePath:  v.URL,
					TextContent: fullText,
					SourceType:  "tiktok_rod_intercept",
					Metadata: map[string]interface{}{
						"comments": v.Comments,
						"author":   v.Author,
						"title":    v.Title,
					},
				}

				data, _ := json.Marshal(payload)

				_, err = s.js.Publish("data.text_extracted", data)
				if err != nil {
					log.Printf("Erro ao publicar no NATS: %v", err)
				} else {
					log.Printf("Enviado: %s", v.ID)
					if err := s.dedup.MarkAsSeen(ctx, v.ID); err != nil {
						log.Printf("Erro ao salvar no Redis: %v", err)
					}
				}
			}
		}
	}
}
