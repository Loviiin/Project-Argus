package sources

import "fmt"

type MockSource struct{}

func (m *MockSource) Name() string {
	return "MockSource"
}

func (m *MockSource) Fetch(query string) ([]RawVideoMetadata, error) {
	fmt.Printf("[MockSource] Buscando por #%s...\n", query)
	return []RawVideoMetadata{
		{
			ID:          "mock-video-1",
			Title:       "Mock Video",
			Description: "Descrição de teste",
			URL:         "https://www.youtube.com/shorts/IknOw-k2nB0",
			Author:      "tester",
			Comments:    []RawComment{{Nick: "user1", Text: "nice"}, {Nick: "user2", Text: "cool"}},
		},
	}, nil
}
