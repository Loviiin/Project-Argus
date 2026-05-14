package sources

import (
	"context"
	"net/http"
	"time"

	"discovery/internal/sources/tiktok"

	"github.com/loviiin/project-argus/pkg/dedup"
)

// TikTokHTTPWrapper implementa a interface Source usando o coletor HTTP puro.
// Converte os tipos internos do pacote tiktok para sources.DiscoveredVideo,
// mantendo o mesmo padrão do TikTokWrapper (Rod).
type TikTokHTTPWrapper struct {
	source *tiktok.HTTPSource
}

// NewTikTokHTTPSource cria uma nova instância do coletor HTTP puro do TikTok.
//   - sidecarURL: endereço do serviço de assinatura (ex: http://localhost:8000).
//     Se vazio, utiliza o default http://localhost:8000.
//   - dedup: deduplicador Redis para filtrar vídeos já processados
func NewTikTokHTTPSource(sidecarURL string, dedup *dedup.Deduplicator) Source {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &TikTokHTTPWrapper{
		source: tiktok.NewTikTokHTTPSource(sidecarURL, client, dedup),
	}
}

// Name retorna o nome identificador do source.
func (w *TikTokHTTPWrapper) Name() string {
	return w.source.Name()
}

// Fetch descobre URLs de vídeos e retorna apenas os novos (filtrados pelo Redis).
func (w *TikTokHTTPWrapper) Fetch(ctx context.Context, query string) ([]DiscoveredVideo, error) {
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

// Close libera recursos. O coletor HTTP não mantém estado persistente.
func (w *TikTokHTTPWrapper) Close() error {
	return w.source.Close()
}
