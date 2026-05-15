package tiktok

import (
	"context"
	"fmt"
	"log"

	sharedtiktok "github.com/loviiin/project-argus/pkg/tiktok"
)

// ── Regex e Funções movidos para pkg/tiktok ────────────────────────────────

// ScrapeAggregator faz um GET ao URL de um agregador e extrai links Discord da página.
func ScrapeAggregator(ctx context.Context, s *HTTPSource, pageURL string) []string {
	resp, err := s.httpClient.R().SetContext(ctx).
		SetHeader("User-Agent", s.userAgent).
		SetHeader("Accept", "text/html,*/*").Get(pageURL)
	if err != nil {
		log.Printf("[Discord-Extractor] erro ao aceder %s: %v", pageURL, err)
		return nil
	}

	body := resp.String()

	links := sharedtiktok.ExtractDiscordLinks(body)
	if len(links) > 0 {
		log.Printf("[Discord-Extractor] %d link(s) Discord encontrado(s) em %s: %v", len(links), pageURL, links)
	}
	return links
}

// BuildTikTokURL constrói a URL canónica de um vídeo TikTok.
func BuildTikTokURL(uniqueID, videoID string) string {
	return fmt.Sprintf("https://www.tiktok.com/@%s/video/%s", uniqueID, videoID)
}
