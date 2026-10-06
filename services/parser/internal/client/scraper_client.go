package client

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type ScraperDiscordClient struct {
	httpClient   *http.Client
	proxyEnabled bool
	rdb          *redis.Client
}

func NewScraperDiscordClient(proxyURLStr string, rdb *redis.Client) *ScraperDiscordClient {
	transport := &http.Transport{}
	proxyEnabled := false

	if proxyURLStr != "" {
		proxyURL, err := url.Parse(proxyURLStr)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
			proxyEnabled = true
		}
	}

	return &ScraperDiscordClient{
		httpClient: &http.Client{
			Timeout:   15 * time.Second,
			Transport: transport,
		},
		proxyEnabled: proxyEnabled,
		rdb:          rdb,
	}
}

func (c *ScraperDiscordClient) GetInviteInfo(ctx context.Context, inviteCode string) (*DiscordInviteResponse, error) {
	// A rota HTTP OpenGraph é MUITO mais permissiva, então talvez nem precisemos do circuit breaker.
	// Mas vamos manter com um tempo menor se tomarmos 429 explícito (ex: 5 minutos em vez de 24h)
	circuitKey := "argus:circuit_breaker:discord_scraper"
	if exists, _ := c.rdb.Exists(ctx, circuitKey).Result(); exists > 0 {
		return nil, ErrCircuitOpen
	}

	if c.proxyEnabled {
		fmt.Printf("[Scraper Client] 🌍 Fazendo requisição para %s usando PROXY ROTATIVO!\n", inviteCode)
	} else {
		fmt.Printf("[Scraper Client] 🕵️ Fazendo requisição para %s via OpenGraph (IP Local)\n", inviteCode)
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	jitterMs := r.Intn(1000) + 500
	time.Sleep(time.Duration(jitterMs) * time.Millisecond)

	url := fmt.Sprintf("https://discord.com/invite/%s", inviteCode)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("erro criar request http: %w", err)
	}

	// Simulamos um crawler como o Twitterbot ou Discordbot, que forçam o Discord a devolver os meta tags do OpenGraph
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Discordbot/2.0; +https://discordapp.com)")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na request http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		c.rdb.Set(ctx, circuitKey, "1", 5*time.Minute) // Bloqueio curto de 5 min
		return nil, fmt.Errorf("rate limited (muitas requisições no scraper)")
	}

	html, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler html: %w", err)
	}
	content := string(html)

	// Validar se o convite existe ou é genérico
	if strings.Contains(content, "Invite Invalid") || strings.Contains(content, "inválido ou expirou") {
		return nil, fmt.Errorf("convite inválido ou expirado")
	}

	var name, icon, guildID string
	var memberCount int

	// Extrair og:title -> Nome
	reTitle := regexp.MustCompile(`<meta property="og:title" content="([^"]+)"`)
	if m := reTitle.FindStringSubmatch(content); len(m) > 1 {
		name = strings.Replace(m[1], "Join the ", "", 1)
		name = strings.Replace(name, " Discord Server!", "", 1)
		name = strings.TrimSpace(name)
	}

	// Extrair og:description -> Membros
	reDesc := regexp.MustCompile(`<meta property="og:description" content="([^"]+)"`)
	if m := reDesc.FindStringSubmatch(content); len(m) > 1 {
		desc := m[1]
		// Padrão antigo: "...with 1,234 other members..."
		reMembers1 := regexp.MustCompile(`(?:with|com)\s+([\d.,]+)\s+(?:other members|outros membros)`)
		// Padrão novo: "... | 1,234 members"
		reMembers2 := regexp.MustCompile(`\|\s+([\d.,]+)\s+(?:members|membros)`)
		
		if m2 := reMembers1.FindStringSubmatch(desc); len(m2) > 1 {
			numStr := strings.ReplaceAll(m2[1], ",", "")
			numStr = strings.ReplaceAll(numStr, ".", "")
			fmt.Sscanf(numStr, "%d", &memberCount)
		} else if m2 := reMembers2.FindStringSubmatch(desc); len(m2) > 1 {
			numStr := strings.ReplaceAll(m2[1], ",", "")
			numStr = strings.ReplaceAll(numStr, ".", "")
			fmt.Sscanf(numStr, "%d", &memberCount)
		}
	}

	// Extrair og:image -> Icon e ID
	reImg := regexp.MustCompile(`<meta property="og:image" content="([^"]+)"`)
	if m := reImg.FindStringSubmatch(content); len(m) > 1 {
		icon = strings.Split(m[1], "?")[0] // remover query params

		// Ex: https://cdn.discordapp.com/icons/762391039572574248/a_hash.jpg
		// Ou splashes: https://cdn.discordapp.com/splashes/1495867270319308821/hash.jpg
		reId := regexp.MustCompile(`(icons|splashes)/(\d+)/`)
		if mId := reId.FindStringSubmatch(icon); len(mId) > 2 {
			guildID = mId[2]
		}
	}

	// Se o titulo bater com titulos genéricos da pagina de erro do discord, o convite não existe mais.
	genericTitles := []string{
		"Discord - Group Chat That’s All Fun &amp; Games",
		"Discord - Group Chat That’s All Fun & Games",
		"Discord - Um lugar para conversar",
		"Discord",
	}
	for _, t := range genericTitles {
		if name == t {
			return nil, fmt.Errorf("convite inválido ou expirado (fallback genérico)")
		}
	}

	res := &DiscordInviteResponse{
		Code:                   inviteCode,
		ApproximateMemberCount: memberCount,
	}
	res.Guild.ID = guildID
	res.Guild.Name = name
	res.Guild.Icon = icon // URL completa! Exatamente o que o parser espera no modo scraper

	return res, nil
}
