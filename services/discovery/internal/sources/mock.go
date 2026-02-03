package sources

import "fmt"

type MockSource struct {
	Calls []string
}

func (m *MockSource) Name() string {
	return "MockSource"
}

func (m *MockSource) FetchRecent(hashtag string) ([]VideoItem, error) {
	m.Calls = append(m.Calls, hashtag)
	fmt.Printf("[MockSource] Buscando por #%s...\n", hashtag)

	return []VideoItem{
		{
			URL:      "https://www.youtube.com/shorts/IknOw-k2nB0",
			Platform: "YouTube shorts",
			Author:   "@tester",
		},
		// Adicione mais se quiser testar volume
	}, nil
}
