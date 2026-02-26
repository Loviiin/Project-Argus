package tiktok

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/utils"
	"github.com/go-rod/stealth"
	"github.com/loviiin/project-argus/pkg/config"
	"github.com/loviiin/project-argus/pkg/dedup"
	"github.com/redis/go-redis/v9"
)

const (
	fetchTimeout = 20000 * time.Second
)

// Source é o scraper de discovery do TikTok.
// Responsável APENAS por navegar em hashtag pages e coletar URLs de vídeos.
// Não abre páginas de vídeos individuais — isso é responsabilidade do Scraper Worker.
type Source struct {
	browser  *rod.Browser
	launcher *launcher.Launcher
	dedup    *dedup.Deduplicator
	rdb      *redis.Client
}

const maxVideos = 150

// NewSource cria uma nova instância do TikTok discovery source.
// O browser persiste sessão em ./browser_state_discovery para manter cookies/tokens
// e evitar captchas repetidos na página da hashtag.
func NewSource(dedup *dedup.Deduplicator, rdb *redis.Client) *Source {
	userDataDir := "./browser_state_discovery"

	// Cleanup stale lock files that often cause "Failed to get the debug url"
	// if a previous session crashed or didn't close properly.
	lockFile := filepath.Join(userDataDir, "lockfile")
	activePort := filepath.Join(userDataDir, "DevToolsActivePort")

	if _, err := os.Stat(lockFile); err == nil {
		fmt.Printf("[Discovery] Removendo lockfile antigo: %s\n", lockFile)
		os.Remove(lockFile)
	}
	if _, err := os.Stat(activePort); err == nil {
		fmt.Printf("[Discovery] Removendo DevToolsActivePort antigo: %s\n", activePort)
		os.Remove(activePort)
	}

	l := launcher.New().
		UserDataDir(userDataDir). // Persiste sessão para hashtag pages
		Leakless(false).
		NoSandbox(true).
		Devtools(true)

	cfg := config.LoadConfig()
	if cfg.Browser.Headless {
		l = l.Set("headless", "new") // Para produção (Evasão Anti-Bot)
	} else {
		l = l.Headless(false) // Para desenvolvimento/VNC (Permite ver a tela)
	}

	// Usa browser instalado no sistema se encontrar; senão go-rod baixa Chromium
	if chromePath, found := launcher.LookPath(); found {
		fmt.Printf("[Discovery] Usando browser em: %s\n", chromePath)
		l = l.Bin(chromePath)
	} else {
		fmt.Println("[Discovery] Chrome não encontrado no PATH, o Rod tentará baixar o Chromium...")
	}

	// Tenta lançar o browser com tratamento de erro explícito
	u, err := l.Launch()
	if err != nil {
		log.Printf("[Discovery] ERRO CRÍTICO ao lançar browser principal: %v\n", err)
		// Se falhar de vez, tentamos criar um "novo" launcher limpo como fallback
		log.Println("[Discovery] Tentando lançamento de emergência limpo (sem UserDataDir)...")

		l = launcher.New().
			Leakless(false).
			NoSandbox(true).
			Devtools(true)

		if cfg.Browser.Headless {
			l = l.Set("headless", "new") // Para produção (Evasão Anti-Bot)
		} else {
			l = l.Headless(false) // Para desenvolvimento/VNC (Permite ver a tela)
		}

		if chromePath, found := launcher.LookPath(); found {
			l = l.Bin(chromePath)
		}

		u = l.MustLaunch()
	}

	browser := rod.New().ControlURL(u).MustConnect()

	// Monitor para debug via navegador
	go func() {
		defer utils.Pause()
		browser.ServeMonitor(":9222")
	}()

	return &Source{browser: browser, launcher: l, dedup: dedup, rdb: rdb}
}

func (s *Source) Name() string {
	return "TikTok-Rod-Discovery"
}

// Close fecha o browser de forma limpa garantindo que o processo morra
func (s *Source) Close() error {
	var err error
	if s.browser != nil {
		err = s.browser.Close()
	}
	if s.launcher != nil {
		s.launcher.Cleanup()
		fmt.Println("[Discovery] Processo do browser encerrado via launcher.Cleanup()")
	}
	return err
}

// DiscoveredVideo contém apenas o ID e URL de um vídeo descoberto.
type DiscoveredVideo struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// Fetch navega na hashtag page, coleta links de vídeo, filtra pelo Redis (IsNew),
// e retorna apenas os vídeos ainda não processados.
func (s *Source) Fetch(ctx context.Context, query string) ([]DiscoveredVideo, error) {
	page, err := stealth.Page(s.browser)
	if err != nil {
		return nil, fmt.Errorf("erro criando pagina stealth: %w", err)
	}
	defer page.Close()

	// Watchdog timeout para prevenir memory/tab leaks
	// Força o fechamento da aba se o rod travar em alguma operação síncrona
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
		case <-time.After(5 * time.Minute):
			fmt.Printf("[Discovery] 🚨 Watchdog: Timeout estrito de 5m atingido. Forçando encerramento da aba para %s!\n", query)
			page.Close()
		}
	}()

	start := time.Now()

	// Se a query já é uma URL direta de vídeo, retorna diretamente (sem filtro Redis aqui)
	if strings.Contains(query, "tiktok.com") && strings.Contains(query, "/video/") {
		videoID := extractID(query)
		isProcessed, err := s.dedup.CheckIfProcessed(ctx, "processed_job", videoID)
		if err != nil {
			return nil, fmt.Errorf("erro redis para %s: %w", videoID, err)
		}
		if isProcessed {
			fmt.Printf("[Discovery] skip (já visto): %s\n", videoID)
			s.rdb.Incr(ctx, "argus:metrics:discovery:duplicates")
			return nil, nil
		}
		return []DiscoveredVideo{{ID: videoID, URL: query}}, nil
	}

	tagURL := fmt.Sprintf("https://www.tiktok.com/tag/%s", query)
	fmt.Printf("[Discovery] hashtag: %s\n", tagURL)

	if err := page.Timeout(fetchTimeout).Navigate(tagURL); err != nil {
		return nil, err
	}
	page.Timeout(15 * time.Second).WaitLoad()
	time.Sleep(3 * time.Second)

	if err := page.Reload(); err != nil {
		fmt.Printf("[Discovery] reload error: %v\n", err)
	}
	page.Timeout(15 * time.Second).WaitLoad()
	time.Sleep(2 * time.Second)

	if isCaptchaPresent(page) {
		if err := s.handleCaptcha(page, query); err != nil {
			return nil, fmt.Errorf("captcha: %w", err)
		}
		start = time.Now()
		page.Timeout(15 * time.Second).WaitLoad()
		time.Sleep(3 * time.Second)
	}

	if _, err := page.Timeout(15 * time.Second).Element(`a[href*="/video/"]`); err != nil {
		fmt.Printf("[Discovery] nenhum video detectado ainda: %v\n", err)
	}

	// Scroll para carregar mais vídeos
	for i := 0; i < 8; i++ {
		page.Mouse.Scroll(0, 1200, 1)
		time.Sleep(1500 * time.Millisecond)
		if i == 3 {
			time.Sleep(1 * time.Second)
		}
	}
	page.Eval(`() => window.scrollTo(0, 0)`)
	time.Sleep(1 * time.Second)

	if time.Since(start) > fetchTimeout {
		return nil, fmt.Errorf("timeout ao coletar videos")
	}

	// Coleta os hrefs dos links de vídeo
	rawURLs := s.collectVideoURLs(page)

	rawURLs = unique(rawURLs)
	if len(rawURLs) > maxVideos {
		rawURLs = rawURLs[:maxVideos]
	}

	fmt.Printf("[Discovery] %d URLs únicas encontradas, filtrando pelo Redis...\n", len(rawURLs))

	// Top-of-Funnel: filtra pelo Redis
	var discovered []DiscoveredVideo
	for _, rawURL := range rawURLs {
		videoID := extractID(rawURL)
		if videoID == "" {
			continue
		}

		isProcessed, err := s.dedup.CheckIfProcessed(ctx, "processed_job", videoID)
		if err != nil {
			fmt.Printf("[Discovery] erro redis para %s: %v\n", videoID, err)
			continue
		}
		if isProcessed {
			fmt.Printf("[Discovery] skip (já visto): %s\n", videoID)
			s.rdb.Incr(ctx, "argus:metrics:discovery:duplicates")
			continue
		}

		discovered = append(discovered, DiscoveredVideo{ID: videoID, URL: rawURL})
	}

	fmt.Printf("[Discovery] %d vídeos novos após filtro Redis\n", len(discovered))
	return discovered, nil
}

// collectVideoURLs extrai todas as URLs de vídeo da página da hashtag.
func (s *Source) collectVideoURLs(page *rod.Page) []string {
	videoLinks, err := page.Timeout(5 * time.Second).Elements(`a[href*="/video/"]`)
	if err != nil {
		// Fallback: busca em todos os links
		allLinks, err2 := page.Timeout(5 * time.Second).Elements("a")
		if err2 != nil {
			return nil
		}
		var urls []string
		for _, link := range allLinks {
			href, herr := link.Attribute("href")
			if herr == nil && href != nil && strings.Contains(*href, "/video/") {
				urls = append(urls, *href)
			}
		}
		return urls
	}

	var urls []string
	for _, link := range videoLinks {
		href, err := link.Attribute("href")
		if err == nil && href != nil && strings.Contains(*href, "/video/") {
			urls = append(urls, *href)
		}
	}
	return urls
}

func (s *Source) handleCaptcha(page *rod.Page, ctxStr string) error {
	captchaType := detectCaptchaType(page)
	fmt.Printf("[%s] [Captcha] tipo: %s\n", ctxStr, captchaType)

	var err error
	switch captchaType {
	case CaptchaTypeRotate:
		err = handleRotateCaptcha(page, ctxStr)
	case CaptchaTypePuzzle:
		err = handlePuzzleCaptcha(page)
	default:
		err = waitCaptchaResolution(page, 5*time.Minute)
	}

	if err != nil {
		return fmt.Errorf("captcha: %w", err)
	}

	time.Sleep(3 * time.Second)

	if isCaptchaPresent(page) {
		return ErrCaptcha
	}

	return nil
}

func isCaptchaPresent(page *rod.Page) bool {
	info, _ := page.Info()
	urlStr := ""
	if info != nil {
		urlStr = info.URL
	}

	if strings.Contains(strings.ToLower(urlStr), "captcha") {
		return true
	}

	if has, _, err := page.Has(`iframe[src*="captcha"]`); err == nil && has {
		return true
	}

	strictSelectors := []string{
		".captcha_verify_container",
		".captcha_verify_img_slide",
		"[class*='secsdk-captcha']",
		"[id*='captcha']",
		"div[class*='captcha_verify']",
	}

	if has, _, err := page.Has(strings.Join(strictSelectors, ", ")); err == nil && has {
		return true
	}

	result, err := page.Eval(`() => {
		const text = document.body.innerText.toLowerCase();
		return text.includes('drag the slider') || text.includes('fit the puzzle') || text.includes('captcha');
	}`)

	if err == nil && result.Value.Bool() {
		return true
	}

	return false
}

func unique(strSlice []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range strSlice {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

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

func parseCount(s string) int {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0
	}

	multiplier := 1.0
	if strings.HasSuffix(s, "K") {
		multiplier = 1000.0
		s = strings.TrimSuffix(s, "K")
	} else if strings.HasSuffix(s, "M") {
		multiplier = 1000000.0
		s = strings.TrimSuffix(s, "M")
	} else if strings.HasSuffix(s, "B") {
		multiplier = 1000000000.0
		s = strings.TrimSuffix(s, "B")
	}

	s = strings.ReplaceAll(s, ",", ".") // Tratar vírgulas como decimais

	var val float64
	fmt.Sscanf(s, "%f", &val)
	return int(val * multiplier)
}
