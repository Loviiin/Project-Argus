package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/loviiin/project-argus/pkg/tiktok"
)

// ScrapeJob é o payload recebido do tópico NATS jobs.scrape.
type ScrapeJob struct {
	VideoID  string `json:"video_id"`
	VideoURL string `json:"video_url"`
	Hashtag  string `json:"hashtag"`
	Desc     string `json:"desc"`
	Author   string `json:"author"`
}

// ArtifactPayload é o payload publicado no tópico data.text_extracted.
type ArtifactPayload struct {
	SourcePath  string                 `json:"source_path"`
	TextContent string                 `json:"text_content"`
	AuthorID    string                 `json:"author_id,omitempty"`
	SourceType  string                 `json:"source_type,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Processor encapsula as dependências para processar vídeos.
type Processor struct {
	SidecarURL string
	HttpClient *http.Client
}

func NewProcessor(sidecarURL string) *Processor {
	return &Processor{
		SidecarURL: strings.TrimRight(sidecarURL, "/"),
		HttpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *Processor) ProcessVideo(ctx context.Context, job ScrapeJob) (*ArtifactPayload, error) {
	// 1. Extrair links da descrição (já vem no job)
	descText := job.Desc
	authorID := job.Author
	
	discordLinks := tiktok.ExtractDiscordLinks(descText)
	
	// 2. Buscar comentários via Sidecar (API Evil0ctal)
	log.Printf("[Worker] 💬 Buscando comentários para o vídeo %s...", job.VideoID)
	comments, err := p.fetchVideoComments(ctx, job.VideoID)
	if err != nil {
		log.Printf("[Worker] ⚠️ erro ao buscar comentários para %s: %v", job.VideoID, err)
		// Continuamos apenas com a descrição se falhar
	}

	var commentTexts []string
	for _, c := range comments {
		commentTexts = append(commentTexts, c.Text)
		// Procurar links nos comentários
		links := tiktok.ExtractDiscordLinks(c.Text)
		discordLinks = append(discordLinks, links...)
	}

	// 3. Agregar tudo num texto único para o Parser
	fullText := descText
	if len(commentTexts) > 0 {
		fullText += "\n\n--- COMMENTS ---\n" + strings.Join(commentTexts, "\n")
	}

	// Log formatado
	fmt.Printf("\n[Worker] ✅ Vídeo Processado: %s\n", job.VideoID)
	fmt.Printf("      👤 Autor: @%s\n", authorID)
	fmt.Printf("      📝 Descrição: %s\n", truncate(sanitize(descText), 100))
	fmt.Printf("      💬 Comentários analisados: %d\n", len(comments))
	if len(discordLinks) > 0 {
		fmt.Printf("      🎯 Links Discord encontrados: %v\n", discordLinks)
	}

	// Monta o payload
	payload := &ArtifactPayload{
		SourcePath:  job.VideoURL,
		TextContent: fullText,
		SourceType:  "tiktok_api_full",
		AuthorID:    authorID,
		Metadata: map[string]interface{}{
			"hashtag":       job.Hashtag,
			"video_id":      job.VideoID,
			"author":        authorID,
			"comment_count": len(comments),
			"discord_links": discordLinks,
		},
	}

	return payload, nil
}

type commentData struct {
	Text string `json:"text"`
}

func (p *Processor) fetchVideoComments(ctx context.Context, videoID string) ([]commentData, error) {
	endpoint := fmt.Sprintf("%s/api/tiktok/web/fetch_video_comments?itemId=%s&count=50&cursor=0", p.SidecarURL, videoID)
	
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sidecar retornou status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Comments []commentData `json:"comments"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}

	return envelope.Data.Comments, nil
}

// RandomDelay aplica um delay aleatório entre min e max segundos.
func RandomDelay(minSec, maxSec int) {
	delay := time.Duration(rand.Intn(maxSec-minSec+1)+minSec) * time.Second
	fmt.Printf("[Worker] ⏳ Delay anti-rate-limit: %v\n", delay)
	time.Sleep(delay)
}

// --- Funções auxiliares (movidas do client.go original) ---

// Removed legacy functions

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
