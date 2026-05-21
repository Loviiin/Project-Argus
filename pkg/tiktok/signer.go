package tiktok

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type SignerClient struct {
	client  *http.Client
	baseURL string
}

type SignRequest struct {
	URL string `json:"url"`
}

type SignResponse struct {
	Status string `json:"status"`
	Data   struct {
		SignedURL string `json:"signed_url"`
		Cookies   string `json:"cookies"`
		Navigator struct {
			UserAgent string `json:"user_agent"`
		} `json:"navigator"`
	} `json:"data"`
}

type FetchResponse struct {
	Status     string          `json:"status"`
	HTTPStatus int             `json:"httpStatus"`
	Data       json.RawMessage `json:"data"`
}

func NewSignerClient(baseURL string) *SignerClient {
	return &SignerClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 10 * time.Second, // Timeout maior pois requests no sidecar podem demorar
		},
	}
}

// SignURL gets a signed URL and browser fingerprint required to make the request
func (s *SignerClient) SignURL(ctx context.Context, rawURL string) (*SignResponse, error) {
	reqBody := SignRequest{URL: rawURL}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal sign request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/signature", s.baseURL), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sidecar request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sidecar returned status %d", resp.StatusCode)
	}

	var result SignResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode sidecar response: %w", err)
	}

	if result.Status != "ok" {
		return nil, fmt.Errorf("sidecar internal error")
	}

	return &result, nil
}

// FetchURL uses the sidecar's browser to fetch the URL directly, bypassing bot detection
func (s *SignerClient) FetchURL(ctx context.Context, rawURL string) ([]byte, error) {
	reqBody := SignRequest{URL: rawURL}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal fetch request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/fetch", s.baseURL), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sidecar fetch request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sidecar fetch returned status %d", resp.StatusCode)
	}

	var result FetchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode sidecar fetch response: %w", err)
	}

	if result.Status != "ok" || result.HTTPStatus != 200 {
		return nil, fmt.Errorf("sidecar fetch failed with http status %d", result.HTTPStatus)
	}

	return result.Data, nil
}
