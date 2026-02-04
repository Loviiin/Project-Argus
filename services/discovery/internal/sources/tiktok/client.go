package tiktok

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"
)

const (
	fetchTimeout     = 45 * time.Second
	perVideoTimeout  = 20 * time.Second
	captchaWaitLimit = 60 * time.Second
)

// Source implementa a lógica de scraping do TikTok usando Go-Rod
type Source struct {
	browser *rod.Browser
}

// NewSource cria uma nova instância do scraper TikTok
// Inicializa o browser com stealth mode e devtools habilitado
func NewSource() *Source {
	path, _ := launcher.LookPath()

	l := launcher.New().
		Bin(path).
		Headless(true).
		Devtools(true)

	u := l.MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()

	fmt.Println("Monitor do Go-Rod rodando em: http://localhost:9222")
	go browser.ServeMonitor(":9222")

	return &Source{
		browser: browser,
	}
}

// Name retorna o nome identificador do source
func (s *Source) Name() string {
	return "TikTok-Rod-Full"
}

// Fetch faz: Busca na Tag -> Coleta URLs -> Extrai Comentários (XHR Intercept)
func (s *Source) Fetch(query string) ([]RawVideoMetadata, error) {
	page, err := stealth.Page(s.browser)
	if err != nil {
		return nil, fmt.Errorf("erro criando pagina stealth: %w", err)
	}
	defer page.Close()

	start := time.Now()

	// Se a query é uma URL direta, processa apenas esse vídeo
	if strings.Contains(query, "tiktok.com") {
		fmt.Printf("[Rod] URL direta detectada: %s\n", query)
		meta, err := s.processVideo(query)
		if err != nil {
			return nil, err
		}
		return []RawVideoMetadata{meta}, nil
	}

	// Caso contrário, busca pela tag
	tagURL := fmt.Sprintf("https://www.tiktok.com/tag/%s", query)
	fmt.Printf("[Rod] Navegando para tag: %s\n", tagURL)

	if err := page.Timeout(fetchTimeout).Navigate(tagURL); err != nil {
		return nil, err
	}
	page.Timeout(15 * time.Second).WaitLoad()

	// Aguarda a página renderizar completamente
	fmt.Println("[Rod] Aguardando renderização da página...")
	time.Sleep(3 * time.Second)

	// PRIMEIRO RELOAD: Força reload para garantir que Rod DevTools esteja sincronizado
	fmt.Println("[Rod] Reload preventivo (garante sincronização com DevTools)...")
	if err := page.Reload(); err != nil {
		fmt.Printf("[Rod] Aviso: Erro no reload: %v\n", err)
	}
	page.Timeout(15 * time.Second).WaitLoad()
	time.Sleep(2 * time.Second)

	// Verifica se há captcha na página de listagem
	if isCaptchaPresent(page) {
		fmt.Println("[Rod] CAPTCHA detectado na página de tag.")
		if err := s.handleCaptcha(page); err != nil {
			return nil, fmt.Errorf("falha ao resolver captcha: %w", err)
		}
		// Após resolver captcha, aguarda a página recarregar
		fmt.Println("[Rod] Aguardando página recarregar após captcha...")
		page.Timeout(10 * time.Second).WaitLoad()
		time.Sleep(3 * time.Second)
	}

	// Scroll para carregar mais vídeos
	for i := 0; i < 3; i++ {
		page.Mouse.Scroll(0, 1000, 1)
		time.Sleep(2 * time.Second)
	}

	if time.Since(start) > fetchTimeout {
		return nil, fmt.Errorf("timeout atingido ao coletar vídeos na tag")
	}

	// Coleta todos os links de vídeo
	videoLinks, err := page.Timeout(5 * time.Second).Elements("a")
	if err != nil {
		return nil, fmt.Errorf("erro buscando elementos a: %w", err)
	}

	var urlsToVisit []string
	for _, link := range videoLinks {
		href, err := link.Attribute("href")
		if err == nil && href != nil && strings.Contains(*href, "/video/") {
			urlsToVisit = append(urlsToVisit, *href)
		}
	}

	urlsToVisit = unique(urlsToVisit)
	if len(urlsToVisit) > 5 {
		urlsToVisit = urlsToVisit[:5]
	}

	fmt.Printf("[Rod] Encontrados %d vídeos únicos. Iniciando extração profunda...\n", len(urlsToVisit))
	var results []RawVideoMetadata

	// Processa cada vídeo individualmente
	for _, videoURL := range urlsToVisit {
		time.Sleep(1 * time.Second)

		fmt.Printf("[Rod] Processando: %s\n", videoURL)
		meta, err := s.processVideo(videoURL)
		if err != nil {
			fmt.Printf("Erro processando %s: %v\n", videoURL, err)
			continue
		}
		results = append(results, meta)
	}

	return results, nil
}

// processVideo abre uma aba nova, intercepta a API de comentários e retorna os dados
func (s *Source) processVideo(urlStr string) (RawVideoMetadata, error) {
	page, _ := stealth.Page(s.browser)
	defer page.Close()

	router := page.HijackRequests()

	var capturedComments []string
	// Intercepta chamadas à API de comentários do TikTok
	router.MustAdd("*/comment/list/*", func(ctx *rod.Hijack) {
		ctx.MustLoadResponse()
		body := ctx.Response.Payload().Body
		var resp TikTokAPIResponse
		if err := json.Unmarshal(body, &resp); err == nil {
			for _, c := range resp.Comments {
				cleanText := strings.ReplaceAll(c.Text, "\n", " ")
				capturedComments = append(capturedComments, cleanText)
			}
		}
	})

	go router.Run()

	if err := page.Timeout(perVideoTimeout).Navigate(urlStr); err != nil {
		return RawVideoMetadata{}, err
	}
	page.Timeout(10 * time.Second).WaitLoad()

	// Aguarda a página e as APIs carregarem completamente
	fmt.Println("[Rod] Aguardando carregamento completo do vídeo...")
	time.Sleep(5 * time.Second)

	// Verifica se há captcha na página do vídeo
	if isCaptchaPresent(page) {
		fmt.Println("[Rod] CAPTCHA detectado no vídeo.")
		if err := s.handleCaptcha(page); err != nil {
			return RawVideoMetadata{}, err
		}
		// Após resolver captcha, aguarda a página recarregar
		fmt.Println("[Rod] Aguardando vídeo recarregar após captcha...")
		page.Timeout(10 * time.Second).WaitLoad()
		time.Sleep(3 * time.Second)
	}

	// Scroll para carregar comentários
	go func() {
		time.Sleep(2 * time.Second)
		page.Mouse.Scroll(0, 500, 1)
	}()

	time.Sleep(5 * time.Second)

	// Extrai a descrição do vídeo
	descText := ""
	if el, err := page.Timeout(3 * time.Second).Element("h1"); err == nil {
		descText, _ = el.Text()
	}

	return RawVideoMetadata{
		URL:         urlStr,
		Description: descText,
		Comments:    capturedComments,
		ID:          extractID(urlStr),
	}, nil
}

// handleCaptcha orquestra o processo de resolução do captcha
func (s *Source) handleCaptcha(page *rod.Page) error {
	fmt.Println("🔍 [Captcha] Iniciando resolução de captcha...")

	// Log da URL atual para debug
	info, _ := page.Info()
	if info != nil {
		fmt.Printf("🌐 [Captcha] URL atual: %s\n", info.URL)
	}

	// Tira screenshot para debug (salva em /tmp)
	screenshot, _ := page.Screenshot(true, nil)
	if screenshot != nil {
		timestamp := time.Now().Format("20060102_150405")
		filename := fmt.Sprintf("/tmp/captcha_debug_%s.png", timestamp)
		if err := os.WriteFile(filename, screenshot, 0644); err == nil {
			fmt.Printf("📸 [Captcha] Screenshot salvo: %s\n", filename)
		}
	}

	// 1. Detecta o tipo de captcha
	captchaType := detectCaptchaType(page)
	fmt.Printf("🎯 [Captcha] Tipo detectado: %s\n", captchaType)

	var err error
	switch captchaType {
	case CaptchaTypeRotate:
		// Resolve captcha de rotação usando SadCaptcha
		fmt.Println(" [Captcha] Iniciando resolução ROTATE...")
		err = handleRotateCaptcha(page)
	case CaptchaTypePuzzle:
		// Resolve captcha de puzzle usando Vision/SadCaptcha
		fmt.Println("🧩 [Captcha] Iniciando resolução PUZZLE...")
		err = handlePuzzleCaptcha(page)
	default:
		fmt.Println("  [Captcha] Tipo DESCONHECIDO. Tentando método genérico...")
		// Fallback: tenta o método antigo
		err = s.handleCaptchaFallback(page)
	}

	if err != nil {
		return fmt.Errorf("erro resolvendo captcha: %w", err)
	}

	// Aguarda validação e reload da página
	fmt.Println("⏳ [Captcha] Aguardando validação e recarregamento...")
	time.Sleep(5 * time.Second)

	// Verifica se o captcha foi resolvido
	if isCaptchaPresent(page) {
		return ErrCaptcha
	}

	fmt.Println(" [Captcha] Captcha resolvido com sucesso!")
	return nil
}

// handleCaptchaFallback método antigo para fallback
func (s *Source) handleCaptchaFallback(page *rod.Page) error {
	fmt.Println("[Captcha] Usando método fallback...")

	// Detecta e extrai as imagens do captcha
	images, err := extractCaptchaImages(page)
	if err != nil {
		return fmt.Errorf("erro extraindo imagens do captcha: %w", err)
	}

	fmt.Printf("[Captcha] Imagens extraídas: Background=%s, Piece=%s\n",
		images.BackgroundURL, images.PieceURL)

	// MOCK: Simulamos uma distância fixa
	mockSolution := CaptchaSolution{
		DistanceX: 150.0,
		Success:   true,
	}

	fmt.Printf("[Captcha] Solução MOCK: distância = %.2f pixels\n", mockSolution.DistanceX)

	// Localiza o elemento do slider
	slider, err := findSlider(page)
	if err != nil {
		return fmt.Errorf("erro localizando slider: %w", err)
	}

	// Executa o movimento humanizado do mouse
	if err := DragSlider(page, slider, mockSolution.DistanceX); err != nil {
		return fmt.Errorf("erro arrastando slider: %w", err)
	}

	return nil
}

// isCaptchaPresent tenta detectar páginas de verificação/segurança do TikTok
func isCaptchaPresent(page *rod.Page) bool {
	info, _ := page.Info()
	urlStr := ""
	if info != nil {
		urlStr = info.URL
	}

	// Verifica pela URL
	if strings.Contains(strings.ToLower(urlStr), "verify") ||
		strings.Contains(strings.ToLower(urlStr), "captcha") {
		return true
	}

	// Verifica por iframe de captcha
	if _, err := page.Timeout(2 * time.Second).Element(`iframe[src*="captcha"]`); err == nil {
		return true
	}

	// Verifica por containers de captcha específicos do TikTok
	captchaSelectors := []string{
		".captcha_verify_container",
		".captcha_verify_img_slide",
		"[class*='captcha']",
		"[class*='secsdk-captcha']",
		"[id*='captcha']",
		"div[class*='verify']",
	}

	for _, selector := range captchaSelectors {
		if _, err := page.Timeout(1 * time.Second).Element(selector); err == nil {
			return true
		}
	}

	// Verifica por texto "Drag the slider" específico do captcha
	if _, err := page.Timeout(1*time.Second).ElementR("*", "(?i)(drag.*slider|fit.*puzzle|verify|captcha)"); err == nil {
		return true
	}

	return false
}

// unique remove duplicatas de um slice de strings
func unique(strSlice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range strSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

// extractID extrai o ID do vídeo da URL
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
