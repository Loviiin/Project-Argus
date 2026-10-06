package client

import (
	"context"
	"fmt"
	"log"
	"strings"
)

type HybridDiscordClient struct {
	apiClient     *HTTPDiscordClient
	scraperClient *ScraperDiscordClient
}

func NewHybridDiscordClient(apiClient *HTTPDiscordClient, scraperClient *ScraperDiscordClient) *HybridDiscordClient {
	return &HybridDiscordClient{
		apiClient:     apiClient,
		scraperClient: scraperClient,
	}
}

func (c *HybridDiscordClient) GetInviteInfo(ctx context.Context, inviteCode string) (*DiscordInviteResponse, error) {
	// Tenta a API primeiro
	resp, err := c.apiClient.GetInviteInfo(ctx, inviteCode)
	if err != nil {
		// Se o Circuit Breaker da API estiver aberto, ou acabou de tomar rate limit (429)
		if err == ErrCircuitOpen || strings.Contains(err.Error(), "rate limited") || strings.Contains(err.Error(), "circuit breaker") {
			log.Printf("[Hybrid] API indisponível ou bloqueada. Acionando Fallback para o Scraper no convite %s...\n", inviteCode)
			
			// Fallback instantâneo para o scraper
			scrapeResp, scrapeErr := c.scraperClient.GetInviteInfo(ctx, inviteCode)
			if scrapeErr != nil {
				return nil, fmt.Errorf("fallback scraper também falhou: %w", scrapeErr)
			}
			return scrapeResp, nil
		}
		
		// Se for outro erro (ex: convite inválido/404), não adianta tentar no scraper, repassa o erro
		return nil, err
	}
	
	return resp, nil
}
