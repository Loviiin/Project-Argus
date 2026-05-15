package sources

import (
	"context"

	"discovery/internal/sources/tiktok"

	"github.com/loviiin/project-argus/pkg/dedup"
	"github.com/redis/go-redis/v9"
)

// ── Stage 1: Hashtag Discovery ────────────────────────────────────────────────

// TikTokHTTPWrapper implementa a interface Source usando o coletor por hashtag.
type TikTokHTTPWrapper struct {
	source *tiktok.HTTPSource
}

// NewTikTokHTTPSource cria o coletor de Stage 1 (busca por hashtag).
//   - sidecarURL: Evil0ctal URL
//   - ttwid: cookie de sessão (config.yaml: tiktok.ttwid)
//   - dedup: deduplicador Redis
func NewTikTokHTTPSource(sidecarURL, ttwid string, dedup *dedup.Deduplicator) Source {
	return &TikTokHTTPWrapper{
		source: tiktok.NewTikTokHTTPSource(sidecarURL, ttwid, dedup),
	}
}

func (w *TikTokHTTPWrapper) Name() string { return w.source.Name() }
func (w *TikTokHTTPWrapper) Close() error { return w.source.Close() }
func (w *TikTokHTTPWrapper) Fetch(ctx context.Context, query string) ([]DiscoveredVideo, error) {
	results, err := w.source.Fetch(ctx, query)
	if err != nil {
		return nil, err
	}
	var out []DiscoveredVideo
	for _, r := range results {
		out = append(out, DiscoveredVideo{
			ID:     r.ID,
			URL:    r.URL,
			Desc:   r.Desc,
			Author: r.Author,
		})
	}
	return out, nil
}

// ── Stage 2: User/Target Tracker ──────────────────────────────────────────────

// TikTokUserWrapper implementa a interface Source usando o rastreador de contas.
type TikTokUserWrapper struct {
	source *tiktok.UserSource
}

// NewTikTokUserSource cria o coletor de Stage 2 (rastreamento de contas).
//   - sidecarURL: Evil0ctal URL
//   - seedAccounts: contas iniciais do config.yaml (tiktok.target_accounts)
//   - rdb: Redis (para ler alvos descobertos pelo Stage 1)
//   - dedup: deduplicador
func NewTikTokUserSource(sidecarURL string, seedAccounts []string, rdb *redis.Client, dedup *dedup.Deduplicator) Source {
	return &TikTokUserWrapper{
		source: tiktok.NewTikTokUserSource(sidecarURL, seedAccounts, rdb, dedup),
	}
}

func (w *TikTokUserWrapper) Name() string { return w.source.Name() }
func (w *TikTokUserWrapper) Close() error { return w.source.Close() }
func (w *TikTokUserWrapper) Fetch(ctx context.Context, query string) ([]DiscoveredVideo, error) {
	results, err := w.source.Fetch(ctx, query)
	if err != nil {
		return nil, err
	}
	var out []DiscoveredVideo
	for _, r := range results {
		out = append(out, DiscoveredVideo{
			ID:     r.ID,
			URL:    r.URL,
			Desc:   r.Desc,
			Author: r.Author,
		})
	}
	return out, nil
}
