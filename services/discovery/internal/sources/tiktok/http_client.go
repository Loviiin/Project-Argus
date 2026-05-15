package tiktok

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	req "github.com/imroc/req/v3"

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
	defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"
	httpMaxVideos    = 150
)

// HTTPSource é o coletor HTTP puro do TikTok (Estágio 1 — busca por hashtag).
// Usa tls-client para imitar o fingerprint do Chrome 131.
type HTTPSource struct {
	sidecarURL string
	httpClient *req.Client
	dedup      *dedup.Deduplicator
	userAgent  string
	ttwid      string
}

// NewTikTokHTTPSource cria o coletor HTTP puro (Estágio 1).
//   - sidecarURL: endereço do Evil0ctal (ex: http://localhost:8000)
//   - ttwid: cookie de sessão anónima do TikTok (obtém em tiktok.com > F12 > Cookies)
//   - dedup: deduplicador Redis
func NewTikTokHTTPSource(sidecarURL, ttwid string, dedup *dedup.Deduplicator) *HTTPSource {
	if sidecarURL == "" {
		sidecarURL = "http://localhost:8000"
	}
	sidecarURL = strings.TrimRight(sidecarURL, "/")

	client := req.C().ImpersonateChrome().SetTimeout(30 * time.Second)

	if ttwid != "" {
		log.Printf("[TikTok] ttwid configurado (%d chars) — sessão válida", len(ttwid))
	} else {
		log.Printf("[TikTok] AVISO: ttwid vazio. Configure tiktok.ttwid no config.yaml para melhores resultados.")
	}

	return &HTTPSource{
		sidecarURL: sidecarURL,
		httpClient: client,
		dedup:      dedup,
		userAgent:  defaultUserAgent,
		ttwid:      ttwid,
	}
}
func (s *HTTPSource) Name() string { return "TikTok-Hashtag-Discovery" }
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

	// ── Buscar ttwid (config tem prioridade; homepage como fallback) ──
	if s.ttwid == "" {
		s.ttwid = s.fetchTtwid(ctx)
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
		
		log.Printf("[HTTP-Discovery] 🔍 URL Base TikTok: %s", targetURL)

		// Assinar via Evil0ctal (gerar X-Bogus para esta URL)
		signedURL, signErr := s.signViaEvil0ctal(ctx, targetURL)
		if signErr != nil {
			return nil, fmt.Errorf("erro ao assinar URL (tentativa %d): %w", attempt+1, signErr)
		}
		
		log.Printf("[HTTP-Discovery] ✍️ URL Assinada pelo Evil0ctal: %s", signedURL)

		// GET à API do TikTok
		r := s.httpClient.R().SetContext(ctx).
			SetHeader("User-Agent", s.userAgent).
			SetHeader("Accept", "application/json, text/plain, */*").
			SetHeader("Referer", "https://www.tiktok.com/").
			SetHeader("Accept-Language", "pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7").
			SetHeader("Accept-Encoding", "identity").
			SetHeader("Connection", "keep-alive").
			SetHeader("Sec-Fetch-Site", "same-site").
			SetHeader("Sec-Fetch-Mode", "cors").
			SetHeader("Sec-Fetch-Dest", "empty").
			SetHeader("Sec-Ch-Ua", `"Chromium";v="120", "Google Chrome";v="120", ";Not A Brand";v="99"`).
			SetHeader("Sec-Ch-Ua-Mobile", "?0").
			SetHeader("Sec-Ch-Ua-Platform", `"Windows"`)

		if msToken != "" || s.ttwid != "" {
			cookieStr := ""
			if msToken != "" {
				cookieStr += "msToken=" + msToken + "; "
			}
			if s.ttwid != "" {
				// Se a string já tiver formato de chave=valor (múltiplos cookies copiados do browser)
				if strings.Contains(s.ttwid, "=") {
					cookieStr += s.ttwid
					if !strings.HasSuffix(cookieStr, "; ") && !strings.HasSuffix(cookieStr, ";") {
						cookieStr += "; "
					} else if strings.HasSuffix(cookieStr, ";") {
						cookieStr += " "
					}
				} else {
					cookieStr += "ttwid=" + s.ttwid + "; "
				}
			} else {
				cookieStr += "tt_webid=7000000000000000000; "
			}
			r.SetHeader("Cookie", strings.TrimSpace(cookieStr))
		}

		resp, doErr := r.Get(signedURL)
		if doErr != nil {
			return nil, fmt.Errorf("erro HTTP GET TikTok: %w", doErr)
		}

		log.Printf("[HTTP-Discovery] Tentativa %d: status=%d Content-Length=%s",
			attempt+1, resp.StatusCode, resp.Header.Get("Content-Length"))

		body = resp.Bytes()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API TikTok retornou status %d", resp.StatusCode)
		}

		if len(body) > 0 {
			log.Printf("[HTTP-Discovery] ✅ Dados recebidos (%d bytes) na tentativa %d", len(body), attempt+1)
			break
		}

		newToken := resp.Header.Get("X-Ms-Token")
		if newToken == "" {
			for _, sc := range resp.Header.Values("Set-Cookie") {
				parts := strings.SplitN(sc, ";", 2)
				kv := strings.SplitN(parts[0], "=", 2)
				if len(kv) == 2 && strings.TrimSpace(kv[0]) == "msToken" {
					newToken = kv[1]
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

	// 5. Parse da resposta
	var raw struct {
		StatusCode int `json:"status_code"`
		HasMore    int `json:"has_more"`
		Cursor     int `json:"cursor"`
		Data       []struct {
			Type int `json:"type"`
			Item struct {
				ID     string `json:"id"`
				Desc   string `json:"desc"`
				Author struct {
					UniqueID string `json:"unique_id"`
				} `json:"author"`
			} `json:"item"`
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

	// 6. Extrair vídeos de qualquer formato
	type aweme struct {
		id, uniqueID, shareURL, desc string
	}
	var awemes []aweme

	for _, d := range raw.Data {
		if d.Type == 1 && d.Item.ID != "" {
			awemes = append(awemes, aweme{d.Item.ID, d.Item.Author.UniqueID, "", d.Item.Desc})
		}
	}
	for _, a := range raw.AwemeList {
		if a.AwemeID != "" {
			awemes = append(awemes, aweme{a.AwemeID, a.Author.UniqueID, a.ShareURL, a.Desc})
		}
	}
	for _, item := range raw.ItemList {
		if item.ID != "" {
			awemes = append(awemes, aweme{item.ID, item.Author.UniqueID, "", item.Desc})
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
		discovered = append(discovered, DiscoveredVideo{
			ID:     a.id, 
			URL:    videoURL,
			Desc:   a.desc,
			Author: a.uniqueID,
		})
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

	resp, err := s.httpClient.R().SetContext(ctx).Get(endpoint)
	if err != nil {
		return "", fmt.Errorf("erro ao contactar Evil0ctal em %s: %w", s.sidecarURL, err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Evil0ctal retornou status %d", resp.StatusCode)
	}

	var envelope evil0ctalResponse
	if err := json.Unmarshal(resp.Bytes(), &envelope); err != nil {
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
	resp, err := s.httpClient.R().SetContext(ctx).Get(endpoint)
	if err != nil {
		return "", fmt.Errorf("erro ao contactar Evil0ctal para msToken: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Evil0ctal msToken retornou status %d", resp.StatusCode)
	}

	var envelope evil0ctalResponse
	if err := json.Unmarshal(resp.Bytes(), &envelope); err != nil {
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
	if !strings.HasPrefix(query, "#") {
		query = "#" + query
	}
	params.Set("keyword", query)
	params.Set("count", "30")
	params.Set("offset", "0")
	params.Set("cursor", "0")
	params.Set("search_source", "normal_search")
	params.Set("from_page", "search")

	return "https://www.tiktok.com/api/search/general/full/?" + params.Encode()
}

// fetchTtwid tenta buscar o cookie ttwid da página inicial do TikTok.
func (s *HTTPSource) fetchTtwid(ctx context.Context) string {
	resp, err := s.httpClient.R().SetContext(ctx).
		SetHeader("User-Agent", s.userAgent).
		SetHeader("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8").
		SetHeader("Accept-Language", "pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7").Get("https://www.tiktok.com/")
	if err != nil {
		return ""
	}

	for _, sc := range resp.Header.Values("Set-Cookie") {
		parts := strings.SplitN(sc, ";", 2)
		kv := strings.SplitN(parts[0], "=", 2)
		if len(kv) == 2 && strings.TrimSpace(kv[0]) == "ttwid" {
			log.Printf("[HTTP-Discovery] ttwid capturado: %s", kv[1])
			return kv[1]
		}
	}
	log.Printf("[HTTP-Discovery] ttwid não encontrado no request inicial")
	return ""
}
