package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type SupabaseClient struct {
	URL string
	Key string
}

func NewSupabaseClient(url, key string) *SupabaseClient {
	return &SupabaseClient{
		URL: url,
		Key: key,
	}
}

// SendArtifact Webhook envia o artefato extraído para a tabela do Supabase.
// A tabela de destino será chamada "artifacts" por padrão na REST API.
func (s *SupabaseClient) SendArtifact(inviteCode, serverName, sourceUrl, rawOcr, authorId string, isReply bool) error {
	if s.URL == "" || s.Key == "" {
		return nil // Supabase não configurado, ignorar webhook.
	}

	payload := map[string]interface{}{
		"discord_invite_code": inviteCode,
		"discord_server_name": serverName,
		"source_url":          sourceUrl,
		"raw_ocr_text":        rawOcr,
		"author_id":           authorId,
		"is_reply":            isReply,
	}

	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/rest/v1/artifacts", s.URL), bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("apikey", s.Key)
	req.Header.Set("Authorization", "Bearer "+s.Key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "resolution=merge-duplicates") // Para UPSERT funcionar nativamente

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("supabase retornou status %d", resp.StatusCode)
	}

	return nil
}
