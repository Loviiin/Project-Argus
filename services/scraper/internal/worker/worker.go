package worker

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
	"github.com/loviiin/project-argus/pkg/captcha"
)

// ScrapeJob é o payload recebido do tópico NATS jobs.scrape.
type ScrapeJob struct {
	VideoID  string `json:"video_id"`
	VideoURL string `json:"video_url"`
	Hashtag  string `json:"hashtag"`
}

// ArtifactPayload é o payload publicado no tópico data.text_extracted.
type ArtifactPayload struct {
	SourcePath  string                 `json:"source_path"`
	TextContent string                 `json:"text_content"`
	AuthorID    string                 `json:"author_id,omitempty"`
	SourceType  string                 `json:"source_type,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// RawComment representa um comentário extraído.
type RawComment struct {
	Nick string `json:"nick"`
	Text string `json:"text"`
}

// TikTokAPIResponse representa a resposta da API interna do TikTok.
type TikTokAPIResponse struct {
	Comments []struct {
		Text string `json:"text"`
		User struct {
			UniqueId string `json:"unique_id"`
		} `json:"user"`
	} `json:"comments"`
}

const (
	perVideoTimeout     = 20 * time.Second
	MaxCommentsPerVideo = 200 // Limita o numero maximo de comentários para não travar em vídeos virais
)

// ProcessVideo abre a página de um vídeo, intercepta a API de comentários,
// extrai dados e retorna o payload para publicação.
func ProcessVideo(browser *rod.Browser, job ScrapeJob) (*ArtifactPayload, error) {
	page, err := stealth.Page(browser)
	if err != nil {
		return nil, fmt.Errorf("erro criando pagina stealth: %w", err)
	}
	defer page.Close()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
		case <-time.After(8 * time.Minute):
			fmt.Printf("[Worker] 🚨 Watchdog: Timeout estrito de 8m atingido. Forçando encerramento da aba para %s!\n", job.VideoID)
			page.Close()
		}
	}()

	router := page.HijackRequests()
	defer router.Stop()

	// Cliente HTTP com timeout para evitar que LoadResponse bloqueie para sempre as rotinas do router
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	var mu sync.Mutex
	var capturedComments []RawComment

	router.MustAdd("*/comment/list/*", func(ctx *rod.Hijack) {
		err := ctx.LoadResponse(httpClient, true)
		if err != nil {
			return
		}
		body := ctx.Response.Payload().Body
		var resp TikTokAPIResponse
		if err := json.Unmarshal(body, &resp); err == nil {
			mu.Lock()
			for _, c := range resp.Comments {
				if len(capturedComments) >= MaxCommentsPerVideo {
					break
				}
				capturedComments = append(capturedComments, RawComment{
					Nick: c.User.UniqueId,
					Text: strings.ReplaceAll(c.Text, "\n", " "),
				})
			}
			mu.Unlock()
		}
	})

	router.MustAdd("*/comment/reply/list/*", func(ctx *rod.Hijack) {
		err := ctx.LoadResponse(httpClient, true)
		if err != nil {
			return
		}
		body := ctx.Response.Payload().Body
		var resp TikTokAPIResponse
		if err := json.Unmarshal(body, &resp); err == nil {
			mu.Lock()
			for _, c := range resp.Comments {
				if len(capturedComments) >= MaxCommentsPerVideo {
					break
				}
				capturedComments = append(capturedComments, RawComment{
					Nick: c.User.UniqueId,
					Text: "[reply] " + strings.ReplaceAll(c.Text, "\n", " "),
				})
			}
			mu.Unlock()
		}
	})

	go router.Run()

	// Navega para o vídeo
	if err := page.Timeout(perVideoTimeout).Navigate(job.VideoURL); err != nil {
		return nil, fmt.Errorf("erro navegando para %s: %w", job.VideoURL, err)
	}
	page.Timeout(10 * time.Second).WaitLoad()
	time.Sleep(2 * time.Second)

	// Reload para garantir carregamento completo
	if err := page.Reload(); err != nil {
		fmt.Printf("[Worker] reload error: %v\n", err)
	} else {
		page.Timeout(10 * time.Second).WaitLoad()
	}
	time.Sleep(3 * time.Second)

	// Helper inline para não repetir código
	handleCaptchaIfNeeded := func(ctx string) error {
		if captcha.IsCaptchaPresent(page) {
			fmt.Printf("[Worker] Captcha detectado (%s)! Iniciando Shadow Collector...\n", ctx)
			if err := captcha.RunShadowCollector(page, "../discovery/dataset/rotation_captcha", "scraper"); err != nil {
				fmt.Printf("[Worker] Shadow collector falhou ou deu timeout: %v\n", err)
			}
			if captcha.IsCaptchaPresent(page) {
				if err := waitCaptchaResolution(page, 5*time.Minute); err != nil {
					return fmt.Errorf("falha aguardando resolução manual: %w", err)
				}
			}
			page.Timeout(10 * time.Second).WaitLoad()
			time.Sleep(3 * time.Second)
		}
		return nil
	}

	if err := handleCaptchaIfNeeded("load_inicial"); err != nil {
		return nil, err
	}

	// Clica no botão de comentários
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
				break
			}
		}
	}

	time.Sleep(2 * time.Second)

	// Pegar quantidade de comentários para ajustar scroll
	commentCount := 0
	if el, err := page.Timeout(2 * time.Second).Element(`strong[data-e2e="comment-count"], span[data-e2e="comment-count"]`); err == nil {
		if text, err := el.Text(); err == nil {
			commentCount = parseCount(text)
		}
	}

	passes := 4
	if commentCount > 20 {
		passes = 4 + (commentCount-20)/10
		if passes > 20 {
			passes = 20
		}
	}

	fmt.Printf("[Worker] 💬 Previstos %d comentários. Scroll passes: %d\n", commentCount, passes)

	// Scroll no painel de comentários e clica em replies
	for pass := 0; pass < passes; pass++ {
		mu.Lock()
		currentLen := len(capturedComments)
		mu.Unlock()

		if currentLen >= MaxCommentsPerVideo {
			fmt.Printf("[Worker] 🛑 Limite de %d comentários atingido. Interrompendo scroll...\n", MaxCommentsPerVideo)
			break
		}

		time.Sleep(1500 * time.Millisecond)
		page.Eval(`() => {
			const panel = document.querySelector(
				'[data-e2e="comment-list"], [class*="DivCommentListContainer"], [class*="CommentListScroller"]'
			);
			if (panel) { panel.scrollTop += 800; }
			else { window.scrollBy(0, 400); }
		}`)
		time.Sleep(3 * time.Second)

		replyBtns, _ := page.Elements(
			`[data-e2e="view-more-replies"], [class*="SpanViewMoreReply"], span[class*="view-more"], [class*="DivViewRepliesContainer"]`,
		)

		fmt.Printf("[Worker]   Passo %d/%d - Clicando em %d respostas...\n", pass+1, passes, len(replyBtns))
		for _, btn := range replyBtns {
			_, _ = btn.Eval("() => this.click()")
			time.Sleep(200 * time.Millisecond)
		}

		// O TikTok pode jogar Captchas durante o clique em "View Replies" ou no Scroll excessivo
		if err := handleCaptchaIfNeeded(fmt.Sprintf("loop_comentarios_%d", pass+1)); err != nil {
			return nil, err
		}
	}

	time.Sleep(3 * time.Second)

	descText := extractDescription(page)

	// Log formatado
	fmt.Printf("\n[Worker] ✅ Vídeo Processado: %s\n", job.VideoID)
	fmt.Printf("      📝 Descrição: %s\n", truncate(sanitize(descText), 100))
	fmt.Printf("      💬 Comentários: %d\n", len(capturedComments))
	for i, c := range capturedComments {
		if i >= 3 {
			if len(capturedComments) > 3 {
				fmt.Printf("      ... e mais %d comentários\n", len(capturedComments)-3)
			}
			break
		}
		fmt.Printf("      [%d] @%s: %s\n", i+1, c.Nick, truncate(sanitize(c.Text), 60))
	}

	// Monta o payload
	var commentLines []string
	for _, c := range capturedComments {
		commentLines = append(commentLines, "@"+c.Nick+": "+c.Text)
	}
	fullText := descText + "\n" + strings.Join(commentLines, "\n")

	// Converte []RawComment para []interface{} para metadata
	commentsInterface := make([]interface{}, len(capturedComments))
	for i, c := range capturedComments {
		commentsInterface[i] = c
	}

	payload := &ArtifactPayload{
		SourcePath:  job.VideoURL,
		TextContent: fullText,
		SourceType:  "tiktok_rod_intercept",
		Metadata: map[string]interface{}{
			"comments": commentsInterface,
			"hashtag":  job.Hashtag,
			"video_id": job.VideoID,
		},
	}

	return payload, nil
}

// RandomDelay aplica um delay aleatório entre min e max segundos.
func RandomDelay(minSec, maxSec int) {
	delay := time.Duration(rand.Intn(maxSec-minSec+1)+minSec) * time.Second
	fmt.Printf("[Worker] ⏳ Delay anti-rate-limit: %v\n", delay)
	time.Sleep(delay)
}

// --- Funções auxiliares (movidas do client.go original) ---

func extractDescription(page *rod.Page) string {
	for _, sel := range []string{
		`[data-e2e="browse-video-desc"]`,
		`[data-e2e="video-desc"]`,
		`[data-e2e="new-desc-paragraph"]`,
	} {
		if el, err := page.Timeout(2 * time.Second).Element(sel); err == nil {
			if text, err := el.Text(); err == nil && text != "" {
				return sanitize(text)
			}
		}
	}

	if el, err := page.Timeout(2 * time.Second).Element(`h1`); err == nil {
		if text, err := el.Text(); err == nil && text != "" {
			return sanitize(text)
		}
	}

	if el, err := page.Timeout(1 * time.Second).Element(`meta[property="og:description"]`); err == nil {
		if content, err := el.Attribute("content"); err == nil && content != nil && *content != "" {
			return sanitize(*content)
		}
	}

	return ""
}

// waitCaptchaResolution aguarda o captcha ser resolvido manualmente via VNC.
func waitCaptchaResolution(page *rod.Page, timeout time.Duration) error {
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout aguardando resolução do captcha")
		}
		if !captcha.IsCaptchaPresent(page) {
			fmt.Println("[Worker] ✅ Captcha resolvido!")
			return nil
		}
		time.Sleep(3 * time.Second)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func sanitize(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
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

	s = strings.ReplaceAll(s, ",", ".")

	var val float64
	fmt.Sscanf(s, "%f", &val)
	return int(val * multiplier)
}
