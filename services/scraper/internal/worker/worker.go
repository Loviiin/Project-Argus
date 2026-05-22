package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/loviiin/project-argus/pkg/config"
	"github.com/loviiin/project-argus/pkg/tiktok"
	"github.com/nats-io/nats.go"
)

// ScrapeJob é o payload recebido do tópico NATS jobs.scrape.
type ScrapeJob struct {
	VideoID  string `json:"video_id"`
	VideoURL string `json:"video_url"`
	Hashtag  string `json:"hashtag"`
	Desc     string `json:"desc"`
	Author   string `json:"author"`
	Cursor   int    `json:"cursor"`
	Count    int    `json:"count"`
}

// Processor encapsula as dependências para processar vídeos via Sidecar HTTP API.
type Processor struct {
	Config  *config.Config
	signer  *tiktok.SignerClient
	db      *sql.DB
	js      nats.JetStreamContext
}

func NewProcessor(cfg *config.Config, signer *tiktok.SignerClient, db *sql.DB, js nats.JetStreamContext) *Processor {
	return &Processor{
		Config:  cfg,
		signer:  signer,
		db:      db,
		js:      js,
	}
}

func (p *Processor) Close() {
	if p.db != nil {
		p.db.Close()
	}
}

func (p *Processor) ProcessVideo(ctx context.Context, job ScrapeJob) error {
	log.Printf("[Worker] 🚀 Iniciando extração do vídeo %s (Cursor: %d) via Sidecar", job.VideoID, job.Cursor)

	if job.Count == 0 {
		job.Count = 20
	}

	apiURL := fmt.Sprintf("https://www.tiktok.com/api/comment/list/?WebIdLastTime=%d&aid=1988&app_language=pt-BR&app_name=tiktok_web&aweme_id=%s&browser_language=pt-BR&browser_name=Mozilla&browser_online=true&browser_platform=MacIntel&browser_version=5.0&channel=tiktok_web&cookie_enabled=true&count=%d&cursor=%d&device_id=7520531026079925774&device_platform=web_pc&focus_state=true&history_len=2&is_fullscreen=false&is_page_visible=true&language=pt-BR&os=mac&priority_region=BR&region=BR&screen_height=1080&screen_width=1920&tz_name=America/Sao_Paulo&webcast_language=pt-BR",
		time.Now().Unix(), job.VideoID, job.Count, job.Cursor)

	// 1. Obter a assinatura via Sidecar
	signResp, err := p.signer.SignURL(ctx, apiURL)
	if err != nil {
		return fmt.Errorf("falha na assinatura (Sidecar): %w", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// 2. Executar a requisição GET
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signResp.Data.SignedURL, nil)
	if err != nil {
		return fmt.Errorf("erro ao criar requisição HTTP: %w", err)
	}

	req.Header.Set("User-Agent", signResp.Data.Navigator.UserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://www.tiktok.com/")
	req.Header.Set("Cookie", signResp.Data.Cookies)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("falha na requisição direta: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	// Tratamento de Rate Limit ou Shadowban com Fallback
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden || len(bodyBytes) == 0 {
		log.Printf("[Worker] 🚨 Rate limit ou Shadowban (Status: %d). Tentando fallback /fetch...", resp.StatusCode)
		
		fallbackBytes, fetchErr := p.signer.FetchURL(ctx, apiURL)
		if fetchErr != nil {
			return fmt.Errorf("HTTP %d e falha no fallback /fetch: %w", resp.StatusCode, fetchErr)
		}
		bodyBytes = fallbackBytes
		log.Printf("[Worker] ✅ Fallback /fetch bem-sucedido para vídeo %s", job.VideoID)
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status HTTP inesperado: %d", resp.StatusCode)
	}

	// 6. Decodificar JSON
	var tiktokResp struct {
		Comments []struct {
			Cid               string `json:"cid"`
			Text              string `json:"text"`
			DiggCount         int    `json:"digg_count"`
			ReplyCommentTotal int    `json:"reply_comment_total"`
			User              struct {
				Uid      string `json:"uid"`
				Nickname string `json:"nickname"`
			} `json:"user"`
			CreateTime int64 `json:"create_time"`
		} `json:"comments"`
		HasMore int `json:"has_more"`
		Cursor  int `json:"cursor"`
	}

	if err := json.Unmarshal(bodyBytes, &tiktokResp); err != nil {
		log.Printf("[Worker] 🚨 Não foi possível decodificar JSON. Body: %s", string(bodyBytes))
		return fmt.Errorf("falha ao parsear JSON: %w", err)
	}

	if len(tiktokResp.Comments) == 0 {
		limit := 500
		if len(bodyBytes) < 500 {
			limit = len(bodyBytes)
		}
		log.Printf("[Worker] ⚠️ O JSON não contém comentários! Raw: %s", string(bodyBytes[:limit]))
	}

	// 7. Inserção no PostgreSQL com UPSERT
	insertedCount := 0
	for _, c := range tiktokResp.Comments {
		// UPSERT no PostgreSQL
		query := `
			INSERT INTO comments (cid, aweme_id, text, digg_count, reply_comment_total, uid, nickname, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, to_timestamp($8), NOW())
			ON CONFLICT (cid) DO UPDATE SET
				digg_count = EXCLUDED.digg_count,
				reply_comment_total = EXCLUDED.reply_comment_total,
				updated_at = NOW()
		`
		_, err := p.db.ExecContext(ctx, query,
			c.Cid, job.VideoID, c.Text, c.DiggCount, c.ReplyCommentTotal, c.User.Uid, c.User.Nickname, c.CreateTime)
		if err != nil {
			log.Printf("[Worker] ⚠️ Erro ao inserir comentário %s: %v", c.Cid, err)
		} else {
			insertedCount++
		}

		// Publicar no NATS para o Parser caso o comentário possa conter um link de Discord
		lowerText := strings.ToLower(c.Text)
		if strings.Contains(lowerText, "discord") || strings.Contains(lowerText, "gg/") {
			ocrMsg := map[string]interface{}{
				"source_path":  fmt.Sprintf("https://www.tiktok.com/@%s/video/%s#comment-%s", c.User.Nickname, job.VideoID, c.Cid),
				"text_content": c.Text,
				"author_id":    c.User.Nickname,
			}
			data, _ := json.Marshal(ocrMsg)
			if _, pubErr := p.js.Publish("data.text_extracted", data); pubErr != nil {
				log.Printf("[Worker] ⚠️ Erro ao publicar comentário no NATS para o parser: %v", pubErr)
			}
		}
	}

	log.Printf("[Worker] ✅ Inseridos/Atualizados %d comentários (Vídeo: %s)", insertedCount, job.VideoID)

	// 8. Paginação
	if tiktokResp.HasMore == 1 {
		log.Printf("[Worker] ⏭️ Vídeo %s possui mais páginas. Publicando Cursor %d no NATS...", job.VideoID, tiktokResp.Cursor)
		
		nextJob := job
		nextJob.Cursor = tiktokResp.Cursor
		
		jobData, _ := json.Marshal(nextJob)
		_, err := p.js.Publish("jobs.scrape", jobData)
		if err != nil {
			return fmt.Errorf("falha ao publicar próxima página no NATS: %w", err)
		}
	}

	return nil
}

// RandomDelay aplica um delay aleatório entre min e max segundos.
func RandomDelay(minSec, maxSec int) {
	delay := time.Duration(rand.Intn(maxSec-minSec+1)+minSec) * time.Second
	time.Sleep(delay)
}
