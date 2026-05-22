package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/loviiin/project-argus/pkg/dedup"
	pkgtiktok "github.com/loviiin/project-argus/pkg/tiktok"
)

// TikTokSignatureSearch implements the Source interface using the sidecar API.
type TikTokSignatureSearch struct {
	dedup  *dedup.Deduplicator
	signer *pkgtiktok.SignerClient
}

// NewTikTokSignatureSearch creates a new instance of the signature-based search source.
func NewTikTokSignatureSearch(sidecarURL string, dedup *dedup.Deduplicator) *TikTokSignatureSearch {
	return &TikTokSignatureSearch{
		dedup:  dedup,
		signer: pkgtiktok.NewSignerClient(sidecarURL),
	}
}

// Name returns the identifier of the source.
func (s *TikTokSignatureSearch) Name() string {
	return "TikTok-Signature-Search"
}

// Close is a no-op since there is no heavy browser to close.
func (s *TikTokSignatureSearch) Close() error {
	return nil
}

type searchResponse struct {
	ItemList []struct {
		Item struct {
			ID string `json:"id"`
		} `json:"item"`
	} `json:"item_list"`
	Data []struct {
		Item struct {
			ID string `json:"id"`
		} `json:"item"`
	} `json:"data"`
	HasMore int `json:"has_more"`
	Cursor  int `json:"cursor"`
}

func extractIDFromURL(url string) string {
	parts := strings.Split(url, "/")
	for i, p := range parts {
		if p == "video" && i+1 < len(parts) {
			return strings.Split(parts[i+1], "?")[0]
		}
	}
	return ""
}

// Fetch searches for a query using the TikTok API through the signature sidecar.
func (s *TikTokSignatureSearch) Fetch(ctx context.Context, query string) ([]DiscoveredVideo, error) {
	// Se a query for uma URL direta de vídeo
	if strings.Contains(query, "tiktok.com") && strings.Contains(query, "/video/") {
		videoID := extractIDFromURL(query)
		if videoID == "" {
			return nil, nil
		}
		isProcessed, _ := s.dedup.CheckIfProcessed(ctx, "processed_job", videoID)
		if isProcessed {
			log.Printf("[Discovery] skip (já visto): %s", videoID)
			return nil, nil
		}
		return []DiscoveredVideo{{ID: videoID, URL: query}}, nil
	}

	searchQuery := strings.TrimPrefix(query, "#")
	apiEndpoint := "https://www.tiktok.com/api/search/general/full/"

	var discovered []DiscoveredVideo
	cursor := 0

	log.Printf("[Discovery] 🔍 Buscando organicamente pela keyword: %s via Sidecar (Até 5 páginas)", searchQuery)

	for page := 0; page < 5; page++ {
		params := url.Values{}
		params.Set("WebIdLastTime", fmt.Sprintf("%d", time.Now().Unix()))
		params.Set("aid", "1988")
		params.Set("app_language", "pt-BR")
		params.Set("app_name", "tiktok_web")
		params.Set("browser_language", "pt-BR")
		params.Set("browser_name", "Mozilla")
		params.Set("browser_online", "true")
		params.Set("browser_platform", "MacIntel")
		params.Set("browser_version", "5.0")
		params.Set("channel", "tiktok_web")
		params.Set("cookie_enabled", "true")
		params.Set("count", "20")
		params.Set("cursor", fmt.Sprintf("%d", cursor))
		params.Set("device_id", "7520531026079925774")
		params.Set("device_platform", "web_pc")
		params.Set("focus_state", "true")
		params.Set("history_len", "2")
		params.Set("is_fullscreen", "false")
		params.Set("is_page_visible", "true")
		params.Set("keyword", searchQuery)
		params.Set("language", "pt-BR")
		params.Set("offset", fmt.Sprintf("%d", cursor))
		params.Set("os", "mac")
		params.Set("priority_region", "BR")
		params.Set("region", "BR")
		params.Set("screen_height", "1080")
		params.Set("screen_width", "1920")
		params.Set("search_id", "1") // 1 = video search
		params.Set("tz_name", "America/Sao_Paulo")
		params.Set("webcast_language", "pt-BR")

		apiURL := apiEndpoint + "?" + params.Encode()

		fetchResp, err := s.signer.FetchURL(ctx, apiURL)
		if err != nil {
			log.Printf("[Discovery] ⚠️ erro no signer fetch na página %d: %v", page, err)
			break
		}

		if fetchResp == nil || len(fetchResp) == 0 {
			log.Printf("[Discovery] ⚠️ resposta vazia da api do tiktok na página %d", page)
			break
		}

		var parsed searchResponse
		if err := json.Unmarshal(fetchResp, &parsed); err != nil {
			log.Printf("[Discovery] ⚠️ erro unmarshal JSON da busca na página %d: %v", page, err)
			break
		}

		items := parsed.Data
		if len(parsed.ItemList) > 0 {
			items = parsed.ItemList
		}

		if len(items) == 0 {
			log.Printf("[Discovery] Nenhum vídeo encontrado na página %d", page)
			break
		}

		newVids := 0
		for _, item := range items {
			videoID := item.Item.ID
			if videoID == "" {
				continue
			}

			isProcessed, err := s.dedup.CheckIfProcessed(ctx, "processed_job", videoID)
			if err != nil {
				log.Printf("[Discovery] erro redis para %s: %v\n", videoID, err)
				continue
			}
			if isProcessed {
				continue
			}

			discovered = append(discovered, DiscoveredVideo{
				ID:  videoID,
				URL: fmt.Sprintf("https://www.tiktok.com/@user/video/%s", videoID),
			})
			newVids++
		}
		
		log.Printf("[Discovery] Página %d: +%d vídeos novos (Total parcial: %d)", page, newVids, len(discovered))

		// Paginação
		if parsed.HasMore == 0 {
			break
		}
		
		cursor = parsed.Cursor
		if cursor == 0 {
			// Se por algum motivo o cursor for 0 mas HasMore for 1, usamos o offset manual
			cursor = (page + 1) * 20
		}

		// Rate limit jitter
		time.Sleep(2*time.Second + time.Duration(page)*500*time.Millisecond)
	}

	log.Printf("[Discovery] 🏁 %d vídeos novos totais coletados para %s via Signature Search", len(discovered), query)
	return discovered, nil
}
