package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type DiscordInviteResponse struct {
	Code  string `json:"code"`
	Guild struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Icon string `json:"icon"`
	} `json:"guild"`
	ApproximateMemberCount int     `json:"approximate_member_count"`
	ExpiresAt              *string `json:"expires_at"` // Pode ser null
}

type DiscordClient struct {
	httpClient  *http.Client
	token       string
	clientRedis *redis.Client
}

func NewDiscordClient(token string, rdb *redis.Client) *DiscordClient {
	return &DiscordClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		token:       token,
		clientRedis: rdb,
	}
}

func (c *DiscordClient) GetInviteInfo(ctx context.Context, inviteCode string) (*DiscordInviteResponse, error) {
	cacheKey := fmt.Sprintf("discord:invite:%s", inviteCode)
	val, err := c.clientRedis.Get(ctx, cacheKey).Result()
	if err == nil {
		var cachedInvite DiscordInviteResponse
		if err := json.Unmarshal([]byte(val), &cachedInvite); err == nil {
			fmt.Printf("[CACHE] Convite %s encontrado no Redis\n", inviteCode)
			return &cachedInvite, nil
		}
	}

	url := fmt.Sprintf("https://discord.com/api/v9/invites/%s?with_counts=true", inviteCode)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ArgusOSINT/1.0)")

	if c.token != "" {
		auth := c.token
		if !(strings.HasPrefix(auth, "Bot ") || strings.HasPrefix(auth, "Bearer ")) {
			auth = "Bot " + auth
		}
		req.Header.Set("Authorization", auth)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("convite inválido ou expirado")
	}
	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited (muitas requisições)")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("erro na api discord: %d", resp.StatusCode)
	}

	var inviteData DiscordInviteResponse
	if err := json.NewDecoder(resp.Body).Decode(&inviteData); err != nil {
		return nil, err
	}

	if jsonData, err := json.Marshal(inviteData); err == nil {
		c.clientRedis.Set(ctx, cacheKey, jsonData, 24*time.Hour)
	}

	return &inviteData, nil
}
