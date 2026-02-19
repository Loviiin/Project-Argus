package tiktok

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

const (
	fetchTimeout    = 90 * time.Second
	perVideoTimeout = 20 * time.Second
)

type Source struct {
	browser *rod.Browser
}

const maxVideos = 15

func NewSource() *Source {
	path, _ := launcher.LookPath()

	l := launcher.New().
		Bin(path).
		Headless(false).
		Devtools(true)

	u := l.MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()

	go browser.ServeMonitor(":9222")

	return &Source{browser: browser}
}

func (s *Source) Name() string {
	return "TikTok-Rod-Full"
}

func (s *Source) Fetch(query string) ([]RawVideoMetadata, error) {
	page, err := stealth.Page(s.browser)
	if err != nil {
		return nil, fmt.Errorf("erro criando pagina stealth: %w", err)
	}
	defer page.Close()

	start := time.Now()

	if strings.Contains(query, "tiktok.com") {
		meta, err := s.processVideo(query)
		if err != nil {
			return nil, err
		}
		return []RawVideoMetadata{meta}, nil
	}

	tagURL := fmt.Sprintf("https://www.tiktok.com/tag/%s", query)
	fmt.Printf("[Rod] tag: %s\n", tagURL)

	if err := page.Timeout(fetchTimeout).Navigate(tagURL); err != nil {
		return nil, err
	}
	page.Timeout(15 * time.Second).WaitLoad()
	time.Sleep(3 * time.Second)

	if err := page.Reload(); err != nil {
		fmt.Printf("[Rod] reload error: %v\n", err)
	}
	page.Timeout(15 * time.Second).WaitLoad()
	time.Sleep(2 * time.Second)

	if isCaptchaPresent(page) {
		if err := s.handleCaptcha(page); err != nil {
			return nil, fmt.Errorf("captcha: %w", err)
		}
		start = time.Now()
		page.Timeout(15 * time.Second).WaitLoad()
		time.Sleep(3 * time.Second)
	}

	if _, err := page.Timeout(15 * time.Second).Element(`a[href*="/video/"]`); err != nil {
		fmt.Printf("[Rod] nenhum video detectado ainda: %v\n", err)
	}

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

	videoLinks, err := page.Timeout(5 * time.Second).Elements(`a[href*="/video/"]`)
	if err != nil {
		allLinks, err2 := page.Timeout(5 * time.Second).Elements("a")
		if err2 != nil {
			return nil, fmt.Errorf("erro buscando links: %w", err2)
		}
		var urlsToVisit []string
		for _, link := range allLinks {
			href, herr := link.Attribute("href")
			if herr == nil && href != nil && strings.Contains(*href, "/video/") {
				urlsToVisit = append(urlsToVisit, *href)
			}
		}
		urlsToVisit = unique(urlsToVisit)
		if len(urlsToVisit) > maxVideos {
			urlsToVisit = urlsToVisit[:maxVideos]
		}
		return s.processVideos(urlsToVisit)
	}

	var urlsToVisit []string
	for _, link := range videoLinks {
		href, err := link.Attribute("href")
		if err == nil && href != nil && strings.Contains(*href, "/video/") {
			urlsToVisit = append(urlsToVisit, *href)
		}
	}
	urlsToVisit = unique(urlsToVisit)
	if len(urlsToVisit) > maxVideos {
		urlsToVisit = urlsToVisit[:maxVideos]
	}

	fmt.Printf("[Rod] %d videos unicos encontrados\n", len(urlsToVisit))
	return s.processVideos(urlsToVisit)
}

func (s *Source) processVideos(urls []string) ([]RawVideoMetadata, error) {
	var results []RawVideoMetadata
	for _, videoURL := range urls {
		time.Sleep(1 * time.Second)
		meta, err := s.processVideo(videoURL)
		if err != nil {
			fmt.Printf("[Rod] erro em %s: %v\n", videoURL, err)
			continue
		}
		results = append(results, meta)
	}
	return results, nil
}

func (s *Source) processVideo(urlStr string) (RawVideoMetadata, error) {
	page, _ := stealth.Page(s.browser)
	defer page.Close()

	router := page.HijackRequests()

	var mu sync.Mutex
	var capturedComments []RawComment

	router.MustAdd("*/comment/list/*", func(ctx *rod.Hijack) {
		ctx.MustLoadResponse()
		body := ctx.Response.Payload().Body
		var resp TikTokAPIResponse
		if err := json.Unmarshal(body, &resp); err == nil {
			mu.Lock()
			for _, c := range resp.Comments {
				capturedComments = append(capturedComments, RawComment{
					Nick: c.User.UniqueId,
					Text: strings.ReplaceAll(c.Text, "\n", " "),
				})
			}
			mu.Unlock()
		}
	})

	router.MustAdd("*/comment/reply/list/*", func(ctx *rod.Hijack) {
		ctx.MustLoadResponse()
		body := ctx.Response.Payload().Body
		var resp TikTokAPIResponse
		if err := json.Unmarshal(body, &resp); err == nil {
			mu.Lock()
			for _, c := range resp.Comments {
				capturedComments = append(capturedComments, RawComment{
					Nick: c.User.UniqueId,
					Text: "[reply] " + strings.ReplaceAll(c.Text, "\n", " "),
				})
			}
			mu.Unlock()
		}
	})

	go router.Run()

	if err := page.Timeout(perVideoTimeout).Navigate(urlStr); err != nil {
		return RawVideoMetadata{}, err
	}
	page.Timeout(10 * time.Second).WaitLoad()
	time.Sleep(2 * time.Second)

	if err := page.Reload(); err != nil {
		fmt.Printf("[Rod] reload error: %v\n", err)
	} else {
		page.Timeout(10 * time.Second).WaitLoad()
	}
	time.Sleep(3 * time.Second)

	if isCaptchaPresent(page) {
		if err := s.handleCaptcha(page); err != nil {
			return RawVideoMetadata{}, err
		}
		page.Timeout(10 * time.Second).WaitLoad()
		time.Sleep(3 * time.Second)
	}

	commentSelectors := []string{
		`[data-e2e="comment-icon"]`,
		`[data-e2e="browse-comment"]`,
		`button[aria-label*="omment"]`,
		`strong[data-e2e="comment-count"]`,
		`span[data-e2e="comment-count"]`,
	}
	for _, sel := range commentSelectors {
		if el, err := page.Timeout(2 * time.Second).Element(sel); err == nil {
			if err2 := el.Click(proto.InputMouseButtonLeft, 1); err2 == nil {
				fmt.Printf("[Rod] comentarios clicados via: %s\n", sel)
				break
			}
		}
	}

	for pass := 0; pass < 4; pass++ {
		time.Sleep(1500 * time.Millisecond)
		page.Eval(`() => {
			const panel = document.querySelector(
				'[data-e2e="comment-list"], [class*="DivCommentListContainer"], [class*="CommentListScroller"]'
			);
			if (panel) { panel.scrollTop += 800; }
			else { window.scrollBy(0, 400); }
		}`)
		time.Sleep(1 * time.Second)

		replyBtns, _ := page.Elements(
			`[data-e2e="view-more-replies"], [class*="SpanViewMoreReply"], span[class*="view-more"]`,
		)
		for _, btn := range replyBtns {
			_ = btn.Click(proto.InputMouseButtonLeft, 1)
			time.Sleep(500 * time.Millisecond)
		}
	}

	time.Sleep(3 * time.Second)

	descText := extractDescription(page)

	fmt.Printf("[Rod] video=%s desc=%q comentarios=%d\n",
		extractID(urlStr), truncate(descText, 60), len(capturedComments))

	return RawVideoMetadata{
		URL:         urlStr,
		Description: descText,
		Comments:    capturedComments,
		ID:          extractID(urlStr),
	}, nil
}

func extractDescription(page *rod.Page) string {
	for _, sel := range []string{
		`[data-e2e="browse-video-desc"]`,
		`[data-e2e="video-desc"]`,
		`[data-e2e="new-desc-paragraph"]`,
	} {
		if el, err := page.Timeout(2 * time.Second).Element(sel); err == nil {
			if text, err := el.Text(); err == nil && text != "" {
				return text
			}
		}
	}

	if el, err := page.Timeout(2 * time.Second).Element(`h1`); err == nil {
		if text, err := el.Text(); err == nil && text != "" {
			return text
		}
	}

	if el, err := page.Timeout(1 * time.Second).Element(`meta[property="og:description"]`); err == nil {
		if content, err := el.Attribute("content"); err == nil && content != nil && *content != "" {
			return *content
		}
	}

	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func (s *Source) handleCaptcha(page *rod.Page) error {
	captchaType := detectCaptchaType(page)
	fmt.Printf("[Captcha] tipo: %s\n", captchaType)

	var err error
	switch captchaType {
	case CaptchaTypeRotate:
		err = handleRotateCaptcha(page)
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

	if strings.Contains(strings.ToLower(urlStr), "verify") ||
		strings.Contains(strings.ToLower(urlStr), "captcha") {
		return true
	}

	if _, err := page.Timeout(2 * time.Second).Element(`iframe[src*="captcha"]`); err == nil {
		return true
	}

	for _, sel := range []string{
		".captcha_verify_container",
		".captcha_verify_img_slide",
		"[class*='captcha']",
		"[class*='secsdk-captcha']",
		"[id*='captcha']",
		"div[class*='verify']",
	} {
		if _, err := page.Timeout(1 * time.Second).Element(sel); err == nil {
			return true
		}
	}

	if _, err := page.Timeout(1*time.Second).ElementR("*", "(?i)(drag.*slider|fit.*puzzle|verify|captcha)"); err == nil {
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
