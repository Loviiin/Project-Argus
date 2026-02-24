package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrCircuitOpen = errors.New("circuit breaker open")

type HTTPDiscordClient struct {
	httpClient   *http.Client
	proxyEnabled bool
	rdb          *redis.Client
}

func NewHTTPDiscordClient(proxyURLStr string, rdb *redis.Client) *HTTPDiscordClient {
	transport := &http.Transport{}
	proxyEnabled := false

	if proxyURLStr != "" {
		proxyURL, err := url.Parse(proxyURLStr)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
			proxyEnabled = true
		}
	}

	return &HTTPDiscordClient{
		httpClient: &http.Client{
			Timeout:   15 * time.Second,
			Transport: transport,
		},
		proxyEnabled: proxyEnabled,
		rdb:          rdb,
	}
}

func (c *HTTPDiscordClient) GetInviteInfo(ctx context.Context, inviteCode string) (*DiscordInviteResponse, error) {
	circuitKey := "argus:circuit_breaker:discord"
	if exists, _ := c.rdb.Exists(ctx, circuitKey).Result(); exists > 0 {
		return nil, ErrCircuitOpen
	}

	if c.proxyEnabled {
		fmt.Printf("[API Client] 🌍 Fazendo requisição para %s usando PROXY ROTATIVO!\n", inviteCode)
	} else {
		fmt.Printf("[API Client] ⚠️ Fazendo requisição para %s SEM proxy (IP Local)\n", inviteCode)
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	jitterMs := r.Intn(2000) + 1000
	time.Sleep(time.Duration(jitterMs) * time.Millisecond)

	url := fmt.Sprintf("https://discord.com/api/v9/invites/%s?with_counts=true", inviteCode)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("erro criar request http: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na request http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		c.rdb.Set(ctx, circuitKey, "1", 5*time.Minute)
		return nil, fmt.Errorf("rate limited (muitas requisições)")
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("convite inválido ou expirado")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erro http status: %d", resp.StatusCode)
	}

	var result DiscordInviteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("erro parse json: %w", err)
	}

	return &result, nil
}
