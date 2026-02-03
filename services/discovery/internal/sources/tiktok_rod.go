package sources

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"
)

type TikTokRodSource struct {
	browser *rod.Browser
}

func NewTikTokRodSource() *TikTokRodSource {
	path, _ := launcher.LookPath()

	l := launcher.New().
		Bin(path).
		Headless(true).
		Devtools(true)

	u := l.MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()

	fmt.Println("Monitor do Go-Rod rodando em: http://localhost:9222")
	go browser.ServeMonitor(":9222")

	return &TikTokRodSource{
		browser: browser,
	}
}

func (t *TikTokRodSource) Name() string {
	return "TikTok-Rod-Full"
}

// Fetch faz: Busca na Tag -> Coleta URLs -> Extrai Comentários (XHR Intercept)
func (t *TikTokRodSource) Fetch(query string) ([]RawVideoMetadata, error) {
	page, err := stealth.Page(t.browser)
	if err != nil {
		return nil, fmt.Errorf("erro criando pagina stealth: %w", err)
	}
	defer page.Close()
	if strings.Contains(query, "tiktok.com") {
		fmt.Printf("[Rod] URL direta detectada: %s\n", query)
		meta, err := t.processVideoParams(t.browser, query)
		if err != nil {
			return nil, err
		}
		return []RawVideoMetadata{meta}, nil
	}
	tagURL := fmt.Sprintf("https://www.tiktok.com/tag/%s", query)
	fmt.Printf("[Rod] Navegando para tag: %s\n", tagURL)

	if err := page.Navigate(tagURL); err != nil {
		return nil, err
	}
	page.Timeout(15 * time.Second).WaitLoad()
	for i := 0; i < 3; i++ {
		page.Mouse.Scroll(0, 1000, 1)
		time.Sleep(2 * time.Second)
	}
	videoLinks, err := page.Elements("a")
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

	for _, videoURL := range urlsToVisit {
		time.Sleep(1 * time.Second)

		fmt.Printf("[Rod] Processando: %s\n", videoURL)
		meta, err := t.processVideoParams(t.browser, videoURL)
		if err != nil {
			fmt.Printf("Erro processando %s: %v\n", videoURL, err)
			continue
		}
		results = append(results, meta)
	}

	return results, nil
}

// processVideoParams abre uma aba nova, intercepta a API de comentários e retorna os dados
func (t *TikTokRodSource) processVideoParams(browser *rod.Browser, urlStr string) (RawVideoMetadata, error) {
	page, _ := stealth.Page(browser)
	defer page.Close()
	router := page.HijackRequests()

	var capturedComments []string
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
	if err := page.Navigate(urlStr); err != nil {
		return RawVideoMetadata{}, err
	}
	page.Timeout(10 * time.Second).WaitLoad()
	go func() {
		time.Sleep(2 * time.Second)
		page.Mouse.Scroll(0, 500, 1)
	}()
	time.Sleep(5 * time.Second)
	descText := ""
	if el, err := page.Element("h1"); err == nil {
		descText, _ = el.Text()
	}

	return RawVideoMetadata{
		URL:         urlStr,
		Description: descText,
		Comments:    capturedComments,
		ID:          extractID(urlStr),
	}, nil
}

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
