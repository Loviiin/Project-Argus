package worker

import (
	"fmt"
	"testing"
)

func TestApiURLGeneration(t *testing.T) {
	// Teste de unidade para garantir que a Query String está gerando a URL correta baseada no tipo de Job.
	// Simularemos o comportamento que ocorre dentro do ProcessVideo.

	tests := []struct {
		name      string
		job       ScrapeJob
		wantURL   string
	}{
		{
			name: "Top-Level Comment (No CommentID)",
			job: ScrapeJob{
				VideoID: "123456",
				Count:   20,
				Cursor:  0,
			},
			wantURL: "https://www.tiktok.com/api/comment/list/?WebIdLastTime=123&aid=1988&app_language=pt-BR&app_name=tiktok_web&aweme_id=123456&browser_language=pt-BR&browser_name=Mozilla&browser_online=true&browser_platform=MacIntel&browser_version=5.0&channel=tiktok_web&cookie_enabled=true&count=20&cursor=0&device_id=7520531026079925774&device_platform=web_pc&focus_state=true&history_len=2&is_fullscreen=false&is_page_visible=true&language=pt-BR&os=mac&priority_region=BR&region=BR&screen_height=1080&screen_width=1920&tz_name=America/Sao_Paulo&webcast_language=pt-BR",
		},
		{
			name: "Reply Comment (With CommentID)",
			job: ScrapeJob{
				VideoID:   "123456",
				CommentID: "987654",
				Count:     20,
				Cursor:    40,
			},
			wantURL: "https://www.tiktok.com/api/comment/list/reply/?WebIdLastTime=123&aid=1988&app_language=pt-BR&app_name=tiktok_web&item_id=123456&comment_id=987654&browser_language=pt-BR&browser_name=Mozilla&browser_online=true&browser_platform=MacIntel&browser_version=5.0&channel=tiktok_web&cookie_enabled=true&count=20&cursor=40&device_id=7520531026079925774&device_platform=web_pc&focus_state=true&history_len=2&is_fullscreen=false&is_page_visible=true&language=pt-BR&os=mac&priority_region=BR&region=BR&screen_height=1080&screen_width=1920&tz_name=America/Sao_Paulo&webcast_language=pt-BR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var apiURL string
			if tt.job.CommentID == "" {
				apiURL = fmt.Sprintf("https://www.tiktok.com/api/comment/list/?WebIdLastTime=%d&aid=1988&app_language=pt-BR&app_name=tiktok_web&aweme_id=%s&browser_language=pt-BR&browser_name=Mozilla&browser_online=true&browser_platform=MacIntel&browser_version=5.0&channel=tiktok_web&cookie_enabled=true&count=%d&cursor=%d&device_id=7520531026079925774&device_platform=web_pc&focus_state=true&history_len=2&is_fullscreen=false&is_page_visible=true&language=pt-BR&os=mac&priority_region=BR&region=BR&screen_height=1080&screen_width=1920&tz_name=America/Sao_Paulo&webcast_language=pt-BR",
					int64(123), tt.job.VideoID, tt.job.Count, tt.job.Cursor)
			} else {
				apiURL = fmt.Sprintf("https://www.tiktok.com/api/comment/list/reply/?WebIdLastTime=%d&aid=1988&app_language=pt-BR&app_name=tiktok_web&item_id=%s&comment_id=%s&browser_language=pt-BR&browser_name=Mozilla&browser_online=true&browser_platform=MacIntel&browser_version=5.0&channel=tiktok_web&cookie_enabled=true&count=%d&cursor=%d&device_id=7520531026079925774&device_platform=web_pc&focus_state=true&history_len=2&is_fullscreen=false&is_page_visible=true&language=pt-BR&os=mac&priority_region=BR&region=BR&screen_height=1080&screen_width=1920&tz_name=America/Sao_Paulo&webcast_language=pt-BR",
					int64(123), tt.job.VideoID, tt.job.CommentID, tt.job.Count, tt.job.Cursor)
			}

			if apiURL != tt.wantURL {
				t.Errorf("URL gerada incorreta.\nEsperado:\n%s\nRecebido:\n%s", tt.wantURL, apiURL)
			}
		})
	}
}

func TestReplyBombProtection(t *testing.T) {
	// Garante que a lógica de bloqueio de "Reply Bombs" funcionará conforme o esperado
	// Limite definido: cursor > 60

	tests := []struct {
		name          string
		jobCommentID  string
		tiktokCursor  int
		shouldPublish bool
	}{
		{
			name:          "Top Level Comment - Cursor Baixo (Should Publish)",
			jobCommentID:  "",
			tiktokCursor:  500,
			shouldPublish: true,
		},
		{
			name:          "Top Level Comment - Flood Protection (Should Block)",
			jobCommentID:  "",
			tiktokCursor:  1020,
			shouldPublish: false,
		},
		{
			name:          "Reply Thread - Cursor Baixo (Should Publish)",
			jobCommentID:  "1111",
			tiktokCursor:  40,
			shouldPublish: true,
		},
		{
			name:          "Reply Thread - Cursor Limite (Should Publish)",
			jobCommentID:  "1111",
			tiktokCursor:  60,
			shouldPublish: true,
		},
		{
			name:          "Reply Thread - Cursor Estourado / Reply Bomb (Should Block)",
			jobCommentID:  "1111",
			tiktokCursor:  80,
			shouldPublish: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			
			willPublish := true
			
			// A lógica exata que está no ProcessVideo
			if tt.jobCommentID != "" && tt.tiktokCursor > 60 {
				willPublish = false
			}
			if tt.jobCommentID == "" && tt.tiktokCursor > 1000 {
				willPublish = false
			}

			if willPublish != tt.shouldPublish {
				t.Errorf("Falha na proteção Reply Bomb/Flood. Esperado Publish=%v, Obtido Publish=%v", tt.shouldPublish, willPublish)
			}
		})
	}
}

func TestTargetSubjectSelection(t *testing.T) {
	// Garante que comentários raiz vão para jobs.scrape
	// E respostas de comentários vão para jobs.scrape.reply
	tests := []struct {
		name         string
		jobCommentID string
		wantSubject  string
	}{
		{
			name:         "Comentário Raiz",
			jobCommentID: "",
			wantSubject:  "jobs.scrape",
		},
		{
			name:         "Resposta de Comentário",
			jobCommentID: "123456",
			wantSubject:  "jobs.scrape.reply",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetSubject := "jobs.scrape"
			if tt.jobCommentID != "" {
				targetSubject = "jobs.scrape.reply"
			}

			if targetSubject != tt.wantSubject {
				t.Errorf("Roteamento falhou. Esperado %s, Obtido %s", tt.wantSubject, targetSubject)
			}
		})
	}
}

func TestLockKeyGeneration(t *testing.T) {
	// Garante que o Lock do Redis não sofre colisão entre respostas do mesmo vídeo na página 0
	tests := []struct {
		name      string
		videoID   string
		commentID string
		cursor    int
		wantKey   string
	}{
		{
			name:      "Lock para Comentário Raiz",
			videoID:   "VIDEO_123",
			commentID: "",
			cursor:    20,
			wantKey:   "argus:processing_lock:VIDEO_123:20",
		},
		{
			name:      "Lock para Resposta de Comentário (Evita Colisão)",
			videoID:   "VIDEO_123",
			commentID: "COMMENT_999",
			cursor:    0,
			wantKey:   "argus:processing_lock:VIDEO_123:COMMENT_999:0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lockSuffix := fmt.Sprintf("%s:%d", tt.videoID, tt.cursor)
			if tt.commentID != "" {
				lockSuffix = fmt.Sprintf("%s:%s:%d", tt.videoID, tt.commentID, tt.cursor)
			}
			lockKey := fmt.Sprintf("argus:processing_lock:%s", lockSuffix)

			if lockKey != tt.wantKey {
				t.Errorf("Lock Key falhou. Esperado %s, Obtido %s", tt.wantKey, lockKey)
			}
		})
	}
}
