package sources

import (
	"context"

	"discovery/internal/sources/tiktok"

	"github.com/loviiin/project-argus/pkg/dedup"
	"github.com/redis/go-redis/v9"
)

// TikTokWrapper implementa a interface Source convertendo tipos do pacote tiktok.
type TikTokWrapper struct {
	source *tiktok.TikTokRodSource
}

// NewTikTokRodSource cria uma nova instância do scraper TikTok Discovery.
func NewTikTokRodSource(dedup *dedup.Deduplicator, rdb *redis.Client) Source {
	return &TikTokWrapper{
		source: tiktok.NewTikTokRodSource(dedup, rdb),
	}
}

// Name retorna o nome do source
func (w *TikTokWrapper) Name() string {
	return w.source.Name()
}

// Fetch descobre URLs e retorna vídeos novos (já filtrados pelo Redis).
func (w *TikTokWrapper) Fetch(ctx context.Context, query string) ([]DiscoveredVideo, error) {
	results, err := w.source.Fetch(ctx, query)
	if err != nil {
		return nil, err
	}

	// Converte tiktok.DiscoveredVideo → sources.DiscoveredVideo
	var converted []DiscoveredVideo
	for _, r := range results {
		converted = append(converted, DiscoveredVideo{
			ID:  r.ID,
			URL: r.URL,
		})
	}

	return converted, nil
}

// Close encerra o browser interno
func (w *TikTokWrapper) Close() error {
	return w.source.Close()
}
