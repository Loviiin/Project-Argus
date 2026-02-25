package tiktok

import (
	"context"

	"github.com/loviiin/project-argus/pkg/dedup"
)

// TikTokRodSource é um wrapper para manter compatibilidade com o código existente.
type TikTokRodSource struct {
	source *Source
}

// NewTikTokRodSource cria uma nova instância do scraper TikTok Discovery.
func NewTikTokRodSource(dedup *dedup.Deduplicator) *TikTokRodSource {
	return &TikTokRodSource{
		source: NewSource(dedup),
	}
}

// Name retorna o nome identificador do source
func (t *TikTokRodSource) Name() string {
	return t.source.Name()
}

// Fetch descobre URLs de vídeos e retorna apenas os novos (filtrados pelo Redis).
func (t *TikTokRodSource) Fetch(ctx context.Context, query string) ([]DiscoveredVideo, error) {
	return t.source.Fetch(ctx, query)
}

// Close encerra a instância do navegador
func (t *TikTokRodSource) Close() error {
	return t.source.Close()
}
