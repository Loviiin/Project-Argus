package sources

import (
	"discovery/internal/sources/tiktok"
)

// TikTokWrapper implementa a interface Source convertendo tipos
type TikTokWrapper struct {
	source *tiktok.TikTokRodSource
}

// NewTikTokRodSource cria uma nova instância do scraper TikTok
func NewTikTokRodSource() Source {
	return &TikTokWrapper{
		source: tiktok.NewTikTokRodSource(),
	}
}

// Name retorna o nome do source
func (w *TikTokWrapper) Name() string {
	return w.source.Name()
}

// Fetch executa a busca e converte os tipos
func (w *TikTokWrapper) Fetch(query string) ([]RawVideoMetadata, error) {
	results, err := w.source.Fetch(query)
	if err != nil {
		return nil, err
	}

	// Converte os tipos do pacote tiktok para o tipo do pacote sources
	var converted []RawVideoMetadata
	for _, r := range results {
		converted = append(converted, RawVideoMetadata{
			ID:          r.ID,
			Title:       r.Title,
			Description: r.Description,
			URL:         r.URL,
			Author:      r.Author,
			Comments:    r.Comments,
		})
	}

	return converted, nil
}
