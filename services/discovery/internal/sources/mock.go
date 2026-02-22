package sources

import (
	"context"
	"fmt"
)

type MockSource struct{}

func (m *MockSource) Name() string {
	return "MockSource"
}

func (m *MockSource) Fetch(_ context.Context, query string) ([]DiscoveredVideo, error) {
	fmt.Printf("[MockSource] Buscando por #%s...\n", query)
	return []DiscoveredVideo{
		{
			ID:  "mock-video-1",
			URL: "https://www.tiktok.com/@user/video/mock-video-1",
		},
	}, nil
}

func (m *MockSource) Close() error {
	return nil
}
