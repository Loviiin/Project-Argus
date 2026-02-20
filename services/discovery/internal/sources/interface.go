package sources

import "context"

// Source é a interface que qualquer fonte de dados (TikTok, YouTube) deve implementar.
type Source interface {
	Name() string
	// Fetch descobre URLs de vídeos baseadas em um termo de busca.
	// Retorna apenas metadados mínimos (ID + URL) já filtrados pelo Redis.
	Fetch(ctx context.Context, query string) ([]DiscoveredVideo, error)
	// Close encerra e limpa recursos usados pela fonte (ex: navegadores)
	Close() error
}
