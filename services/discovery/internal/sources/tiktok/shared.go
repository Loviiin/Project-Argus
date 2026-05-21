package tiktok

import (
	"net/url"
	"regexp"
	"strings"
)

var videoIDPattern = regexp.MustCompile(`/video/([0-9]+)`)

// DiscoveredVideo contém apenas o ID e URL de um vídeo descoberto.
type DiscoveredVideo struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Desc   string `json:"desc"`
	Author string `json:"author"`
}

// ExtractVideoID extrai o ID numérico do vídeo a partir de uma URL ou valor bruto.
func ExtractVideoID(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	if match := videoIDPattern.FindStringSubmatch(rawURL); len(match) == 2 {
		return match[1]
	}

	if parsedURL, err := url.Parse(rawURL); err == nil {
		if match := videoIDPattern.FindStringSubmatch(parsedURL.Path); len(match) == 2 {
			return match[1]
		}

		parts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	parts := strings.Split(strings.Trim(rawURL, "/"), "/")
	if len(parts) == 0 {
		return ""
	}

	return parts[len(parts)-1]
}

// extractID mantém compatibilidade com o código interno existente.
func extractID(rawURL string) string {
	return ExtractVideoID(rawURL)
}
