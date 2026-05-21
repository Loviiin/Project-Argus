

package sources

import (
	"context"

	"discovery/internal/sources/tiktok"
	"github.com/loviiin/project-argus/pkg/dedup"
)

type TikTokRodAdapter struct {
	rodSource *tiktok.TikTokRodSource
}

func NewTikTokRodSourceAdapter(dedup *dedup.Deduplicator) *TikTokRodAdapter {
	return &TikTokRodAdapter{
		rodSource: tiktok.NewTikTokRodSource(dedup),
	}
}

func (a *TikTokRodAdapter) Name() string {
	return a.rodSource.Name()
}

func (a *TikTokRodAdapter) Fetch(ctx context.Context, query string) ([]DiscoveredVideo, error) {
	vids, err := a.rodSource.Fetch(ctx, query)
	if err != nil {
		return nil, err
	}
	var res []DiscoveredVideo
	for _, v := range vids {
		res = append(res, DiscoveredVideo{
			ID:       v.ID,
			URL:      v.URL,
			Desc:     v.Desc,
			Author:   v.Author,
		})
	}
	return res, nil
}

func (a *TikTokRodAdapter) Close() error {
	return a.rodSource.Close()
}
