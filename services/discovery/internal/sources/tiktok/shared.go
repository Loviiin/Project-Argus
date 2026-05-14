package tiktok

import (
	"net/url"
	"strings"
)

// DiscoveredVideo contém apenas o ID e URL de um vídeo descoberto.
type DiscoveredVideo struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// extractID extrai o ID do vídeo a partir de uma URL do TikTok.
func extractID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err == nil {
		parts := strings.Split(u.Path, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	parts := strings.Split(rawURL, "/")
	return parts[len(parts)-1]
}
