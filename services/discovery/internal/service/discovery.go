package service

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"

	"discovery/internal/repository"
	"discovery/internal/sources"

	"github.com/nats-io/nats.go"
)

type DiscoveryService struct {
	dedup       *repository.Deduplicator
	js          nats.JetStreamContext
	sources     []sources.Source
	concurrency int
}

func NewDiscoveryService(dedup *repository.Deduplicator, js nats.JetStreamContext, srcs []sources.Source, workers int) *DiscoveryService {
	if workers <= 0 {
		workers = 1
	}
	return &DiscoveryService{
		dedup:       dedup,
		js:          js,
		sources:     srcs,
		concurrency: workers,
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
		sem := make(chan struct{}, s.concurrency)
		var wg sync.WaitGroup

		for _, tag := range hashtags {
			wg.Add(1)
			sem <- struct{}{}

			go func(tag string) {
				defer wg.Done()
				defer func() { <-sem }()

				log.Printf("[%s] buscando via %s", tag, src.Name())
				videos, err := src.Fetch(tag)
				if err != nil {
					log.Printf("[%s] erro: %v", tag, err)
					return
				}
				log.Printf("[%s] %d videos encontrados", tag, len(videos))

				for _, v := range videos {
					isNew, err := s.dedup.IsNew(ctx, v.ID)
					if err != nil {
						log.Printf("[%s] erro redis: %v", tag, err)
					}
					if err == nil && !isNew {
						continue
					}

					var commentLines []string
					for _, c := range v.Comments {
						commentLines = append(commentLines, "@"+c.Nick+": "+c.Text)
					}
					fullText := v.Description + "\n" + strings.Join(commentLines, "\n")

					payload := ArtifactPayload{
						SourcePath:  v.URL,
						TextContent: fullText,
						SourceType:  "tiktok_rod_intercept",
						Metadata: map[string]interface{}{
							"comments": v.Comments,
							"author":   v.Author,
							"title":    v.Title,
							"hashtag":  tag,
						},
					}

					data, _ := json.Marshal(payload)
					_, err = s.js.Publish("data.text_extracted", data)
					if err != nil {
						log.Printf("[%s] erro publicar %s: %v", tag, v.ID, err)
					} else {
						log.Printf("[%s] publicado: %s (%d comentarios)", tag, v.ID, len(v.Comments))
						if err := s.dedup.MarkAsSeen(ctx, v.ID); err != nil {
							log.Printf("[%s] erro redis mark: %v", tag, err)
						}
					}
				}
			}(tag)
		}

		wg.Wait()
	}
}
