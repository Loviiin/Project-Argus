package tiktok

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/loviiin/project-argus/pkg/dedup"
)

// ── Structs de resposta da API do Evil0ctal ──────────────────────

// evil0ctalResponse é o envelope padrão de todas as respostas da API.
//
//	{ "code": 200, "router": "/api/...", "data": { ... } }
type evil0ctalResponse struct {
	Code   int             `json:"code"`
	Router string          `json:"router,omitempty"`
	Data   json.RawMessage `json:"data"`
}

// tiktokVideoData representa os dados de um vídeo retornados pelo endpoint
// /api/tiktok_web/fetch_one_video (campo "data" do envelope).
type tiktokVideoData struct {
	ItemInfo struct {
		ItemStruct struct {
			ID     string `json:"id"`
			Desc   string `json:"desc"`
			Author struct {
				UniqueID string `json:"uniqueId"`
				Nickname string `json:"nickname"`
			} `json:"author"`
		} `json:"itemStruct"`
	} `json:"itemInfo"`
	// Formato alternativo (TikTok App API via hybrid)
	AwemeID  string `json:"aweme_id,omitempty"`
	Desc     string `json:"desc,omitempty"`
	ShareURL string `json:"share_url,omitempty"`
	Author   *struct {
		UniqueID string `json:"unique_id"`
		Nickname string `json:"nickname"`
	} `json:"author,omitempty"`
}

// tiktokUserPostData representa a resposta de /api/tiktok_web/fetch_user_post.
type tiktokUserPostData struct {
	ItemList []struct {
		ID     string `json:"id"`
		Desc   string `json:"desc"`
		Author struct {
			UniqueID string `json:"uniqueId"`
		} `json:"author"`
	} `json:"itemList"`
	HasMore bool  `json:"hasMore"`
	Cursor  int64 `json:"cursor"`
}

// xbogusResponse mapeia a resposta de /api/tiktok_web/generate_xbogus.
type xbogusResponse struct {
	// O endpoint retorna { "param": "...", "xbogus": "...", "url": "..." }
	Param  string `json:"param,omitempty"`
	XBogus string `json:"xbogus,omitempty"`
	URL    string `json:"url,omitempty"`
}

// ── Implementação ────────────────────────────────────────────────

const (
	defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	httpMaxVideos    = 150
)

// HTTPSource é o coletor HTTP puro do TikTok.
// Comunica com uma instância local do Douyin_TikTok_Download_API (Evil0ctal)
// que lida internamente com X-Bogus, A-Bogus e msToken.
type HTTPSource struct {
	sidecarURL string       // URL base do Evil0ctal (ex: http://localhost:8000)
	httpClient *http.Client // cliente HTTP com timeout
	dedup      *dedup.Deduplicator
	userAgent  string
}

// NewTikTokHTTPSource cria uma nova instância do coletor HTTP puro.
//   - sidecarURL: endereço da API Evil0ctal (ex: http://localhost:8000)
//   - client: cliente HTTP configurado (se nil, cria um com timeout de 30s)
//   - dedup: deduplicador Redis para filtrar vídeos já processados
func NewTikTokHTTPSource(sidecarURL string, client *http.Client, dedup *dedup.Deduplicator) *HTTPSource {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if sidecarURL == "" {
		sidecarURL = "http://localhost:8000"
	}
	sidecarURL = strings.TrimRight(sidecarURL, "/")

	return &HTTPSource{
		sidecarURL: sidecarURL,
		httpClient: client,
		dedup:      dedup,
		userAgent:  defaultUserAgent,
	}
}

func (s *HTTPSource) Name() string { return "TikTok-HTTP-Discovery" }
func (s *HTTPSource) Close() error { return nil }

// Fetch descobre vídeos do TikTok para a hashtag/query fornecida.
//
// Fluxo:
//  1. Constrói a URL da API Web do TikTok com todos os parâmetros obrigatórios
//  2. Envia ao Evil0ctal /api/tiktok_web/generate_xbogus para assinar
//  3. GET à URL assinada diretamente na API do TikTok
//  4. Parse JSON e extração de aweme_id + URL partilhável
//  5. Dedup pelo Redis
func (s *HTTPSource) Fetch(ctx context.Context, query string) ([]DiscoveredVideo, error) {
	// Caso especial: URL direta de vídeo
	if strings.Contains(query, "tiktok.com") && strings.Contains(query, "/video/") {
		return s.handleDirectURL(ctx, query)
	}

	// ── Passo 1: Buscar msToken inicial via Evil0ctal ──
	msToken, err := s.fetchMsToken(ctx)
	if err != nil {
		log.Printf("[HTTP-Discovery] aviso: não foi possível gerar msToken: %v (continuando sem)", err)
		msToken = ""
	}

	// ── Handshake: TikTok exige 2 passos para a Search API ──
	// 1º request → body vazio, mas Set-Cookie/X-Ms-Token com token real
	// 2º request → usa o token do TikTok e recebe os dados
	var body []byte
	for attempt := 0; attempt < 2; attempt++ {
		// Construir URL com msToken atual
		targetURL := buildChallengeURL(query)
		if msToken != "" {
			targetURL += "&msToken=" + url.QueryEscape(msToken)
		}

		// Assinar via Evil0ctal (gerar X-Bogus para esta URL)
		signedURL, signErr := s.signViaEvil0ctal(ctx, targetURL)
		if signErr != nil {
			return nil, fmt.Errorf("erro ao assinar URL (tentativa %d): %w", attempt+1, signErr)
		}

		// GET à API do TikTok
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
		if reqErr != nil {
			return nil, fmt.Errorf("erro ao criar request: %w", reqErr)
		}
		req.Header.Set("User-Agent", s.userAgent)
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Referer", "https://www.tiktok.com/")
		req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7")
		req.Header.Set("Accept-Encoding", "identity")
		if msToken != "" {
			req.Header.Set("Cookie", fmt.Sprintf("msToken=%s; tt_webid=7000000000000000000", msToken))
		}

		resp, doErr := s.httpClient.Do(req)
		if doErr != nil {
			return nil, fmt.Errorf("erro HTTP GET TikTok: %w", doErr)
		}

		log.Printf("[HTTP-Discovery] Tentativa %d: status=%d Content-Length=%s",
			attempt+1, resp.StatusCode, resp.Header.Get("Content-Length"))

		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API TikTok retornou status %d", resp.StatusCode)
		}

		// Se body não vazio, temos dados!
		if len(body) > 0 {
			log.Printf("[HTTP-Discovery] ✅ Dados recebidos (%d bytes) na tentativa %d", len(body), attempt+1)
			break
		}

		// Body vazio → extrair msToken do TikTok para a próxima tentativa
		newToken := resp.Header.Get("X-Ms-Token")
		if newToken == "" {
			// Tentar extrair do Set-Cookie
			for _, cookie := range resp.Cookies() {
				if cookie.Name == "msToken" {
					newToken = cookie.Value
					break
				}
			}
		}

		if newToken != "" && newToken != msToken {
			log.Printf("[HTTP-Discovery] 🔄 Token atualizado via headers do TikTok (%d chars), tentando novamente...", len(newToken))
			msToken = newToken
		} else {
			log.Printf("[HTTP-Discovery] ❌ Body vazio e sem novo token — abortando")
			return nil, fmt.Errorf("TikTok retornou body vazio sem fornecer novo token")
		}
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("TikTok retornou body vazio após 2 tentativas")
	}

	// Debug: mostrar primeiros 500 chars do body
	preview := string(body)
	if len(preview) > 500 {
		preview = preview[:500]
	}
	log.Printf("[HTTP-Discovery] Body (%d bytes): %s", len(body), preview)

	// 5. Parse da resposta — tenta ambos os formatos conhecidos
	var raw struct {
		StatusCode int    `json:"status_code"`
		HasMore    bool   `json:"has_more"`
		Cursor     int64  `json:"cursor"`
		Data       []struct {
			AwemeInfo struct {
				AwemeID  string `json:"aweme_id"`
				Desc     string `json:"desc"`
				ShareURL string `json:"share_url,omitempty"`
				Author   struct {
					UniqueID string `json:"unique_id"`
				} `json:"author"`
			} `json:"aweme_info"`
		} `json:"data,omitempty"`
		AwemeList []struct {
			AwemeID  string `json:"aweme_id"`
			Desc     string `json:"desc"`
			ShareURL string `json:"share_url,omitempty"`
			Author   struct {
				UniqueID string `json:"unique_id"`
			} `json:"author"`
		} `json:"aweme_list,omitempty"`
		ItemList []struct {
			ID     string `json:"id"`
			Desc   string `json:"desc"`
			Author struct {
				UniqueID string `json:"uniqueId"`
			} `json:"author"`
		} `json:"itemList,omitempty"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("erro ao descodificar JSON: %w", err)
	}

	// 5. Extrair vídeos de qualquer formato
	type aweme struct {
		id, uniqueID, shareURL string
	}
	var awemes []aweme

	for _, d := range raw.Data {
		if d.AwemeInfo.AwemeID != "" {
			awemes = append(awemes, aweme{d.AwemeInfo.AwemeID, d.AwemeInfo.Author.UniqueID, d.AwemeInfo.ShareURL})
		}
	}
	for _, a := range raw.AwemeList {
		if a.AwemeID != "" {
			awemes = append(awemes, aweme{a.AwemeID, a.Author.UniqueID, a.ShareURL})
		}
	}
	for _, item := range raw.ItemList {
		if item.ID != "" {
			awemes = append(awemes, aweme{item.ID, item.Author.UniqueID, ""})
		}
	}

	if len(awemes) == 0 {
		log.Printf("[HTTP-Discovery] Nenhum vídeo na resposta para #%s (status_code=%d)", query, raw.StatusCode)
		return nil, nil
	}
	if len(awemes) > httpMaxVideos {
		awemes = awemes[:httpMaxVideos]
	}

	log.Printf("[HTTP-Discovery] %d vídeos encontrados, filtrando pelo Redis...", len(awemes))

	// 6. Dedup
	var discovered []DiscoveredVideo
	for _, a := range awemes {
		isProcessed, err := s.dedup.CheckIfProcessed(ctx, "processed_job", a.id)
		if err != nil {
			log.Printf("[HTTP-Discovery] erro redis para %s: %v", a.id, err)
			continue
		}
		if isProcessed {
			continue
		}

		videoURL := a.shareURL
		if videoURL == "" {
			videoURL = fmt.Sprintf("https://www.tiktok.com/@%s/video/%s", a.uniqueID, a.id)
		}
		discovered = append(discovered, DiscoveredVideo{ID: a.id, URL: videoURL})
	}

	log.Printf("[HTTP-Discovery] %d vídeos novos após filtro Redis para #%s", len(discovered), query)
	return discovered, nil
}

// handleDirectURL trata o caso em que a query já é uma URL direta de vídeo.
func (s *HTTPSource) handleDirectURL(ctx context.Context, rawURL string) ([]DiscoveredVideo, error) {
	videoID := extractID(rawURL)
	if videoID == "" {
		return nil, fmt.Errorf("não foi possível extrair ID de: %s", rawURL)
	}

	isProcessed, err := s.dedup.CheckIfProcessed(ctx, "processed_job", videoID)
	if err != nil {
		return nil, fmt.Errorf("erro redis para %s: %w", videoID, err)
	}
	if isProcessed {
		log.Printf("[HTTP-Discovery] skip (já visto): %s", videoID)
		return nil, nil
	}

	return []DiscoveredVideo{{ID: videoID, URL: rawURL}}, nil
}

// signViaEvil0ctal usa o endpoint /api/tiktok_web/generate_xbogus da API Evil0ctal
// para obter um X-Bogus válido e retorna a URL final assinada.
func (s *HTTPSource) signViaEvil0ctal(ctx context.Context, targetURL string) (string, error) {
	endpoint := fmt.Sprintf("%s/api/tiktok/web/generate_xbogus?url=%s&user_agent=%s",
		s.sidecarURL,
		url.QueryEscape(targetURL),
		url.QueryEscape(s.userAgent),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("erro ao criar request para Evil0ctal: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("erro ao contactar Evil0ctal em %s: %w", s.sidecarURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Evil0ctal retornou status %d", resp.StatusCode)
	}

	var envelope evil0ctalResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return "", fmt.Errorf("erro ao descodificar resposta Evil0ctal: %w", err)
	}
	if envelope.Code != 200 {
		return "", fmt.Errorf("Evil0ctal code=%d", envelope.Code)
	}

	var xb xbogusResponse
	if err := json.Unmarshal(envelope.Data, &xb); err != nil {
		return "", fmt.Errorf("erro ao parsear xbogus data: %w", err)
	}

	if xb.URL != "" {
		return xb.URL, nil
	}

	// Fallback: construir URL manualmente
	if xb.XBogus != "" {
		return targetURL + "&X-Bogus=" + xb.XBogus, nil
	}

	return "", fmt.Errorf("Evil0ctal não retornou URL assinada nem X-Bogus")
}

// fetchMsToken busca um msToken real via Evil0ctal.
// O msToken é um cookie obrigatório para a maioria dos endpoints do TikTok.
func (s *HTTPSource) fetchMsToken(ctx context.Context) (string, error) {
	endpoint := fmt.Sprintf("%s/api/tiktok/web/generate_real_msToken", s.sidecarURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("erro ao contactar Evil0ctal para msToken: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Evil0ctal msToken retornou status %d", resp.StatusCode)
	}

	var envelope evil0ctalResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return "", fmt.Errorf("erro ao descodificar msToken: %w", err)
	}

	// O campo data pode ser uma string direta ou um objeto com "msToken"
	var token string
	if err := json.Unmarshal(envelope.Data, &token); err != nil {
		// Tenta como objeto
		var obj struct {
			MsToken string `json:"msToken"`
		}
		if err2 := json.Unmarshal(envelope.Data, &obj); err2 != nil {
			return "", fmt.Errorf("formato inesperado de msToken: %s", string(envelope.Data))
		}
		token = obj.MsToken
	}

	if token == "" {
		return "", fmt.Errorf("msToken vazio na resposta")
	}

	log.Printf("[HTTP-Discovery] msToken gerado (%d chars)", len(token))
	return token, nil
}

// buildChallengeURL constrói a URL do endpoint de pesquisa por hashtag/challenge.
// Inclui todos os parâmetros obrigatórios que o TikTok Web valida server-side.
func buildChallengeURL(query string) string {
	params := url.Values{}

	// Identidade da aplicação
	params.Set("aid", "1988")
	params.Set("app_name", "tiktok_web")
	params.Set("channel", "tiktok_web")
	params.Set("device_platform", "web_pc")

	// Localização e idioma
	params.Set("app_language", "pt-BR")
	params.Set("browser_language", "pt-BR")
	params.Set("webcast_language", "pt-BR")
	params.Set("region", "BR")
	params.Set("priority_region", "")
	params.Set("tz_name", "America/Sao_Paulo")

	// Dados do "browser"
	params.Set("browser_name", "Mozilla")
	params.Set("browser_version", "131.0.0.0")
	params.Set("browser_platform", "Win32")
	params.Set("browser_online", "true")
	params.Set("os", "windows")
	params.Set("cookie_enabled", "true")

	// Viewport
	params.Set("screen_width", "1920")
	params.Set("screen_height", "1080")

	// Estado da página
	params.Set("focus_state", "true")
	params.Set("is_fullscreen", "false")
	params.Set("is_page_visible", "true")
	params.Set("history_len", "3")

	// Parâmetros de pesquisa
	params.Set("keyword", query)
	params.Set("count", "30")
	params.Set("offset", "0")
	params.Set("cursor", "0")
	params.Set("search_source", "normal_search")
	params.Set("from_page", "search")

	return "https://www.tiktok.com/api/search/general/full/?" + params.Encode()
}
