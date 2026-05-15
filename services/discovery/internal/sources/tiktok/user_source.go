package tiktok

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	req "github.com/imroc/req/v3"
	"github.com/redis/go-redis/v9"

	"github.com/loviiin/project-argus/pkg/dedup"
	sharedtiktok "github.com/loviiin/project-argus/pkg/tiktok"
)

// ── Constantes ───────────────────────────────────────────────────────────────

const (
	// Redis Set onde o Stage 1 acumula usernames de alvos potenciais
	RedisTargetsKey = "argus:targets:potential"
)

// ── Structs Evil0ctal ─────────────────────────────────────────────────────────

type userProfileResponse struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

type tiktokUserProfile struct {
	UserInfo struct {
		User struct {
			UniqueID  string `json:"uniqueId"`
			Nickname  string `json:"nickname"`
			SecUID    string `json:"secUid"`
			Signature string `json:"signature"` // bio text
			BioLink   struct {
				Link string `json:"link"` // website/linktree URL
			} `json:"bioLink"`
		} `json:"user"`
	} `json:"userInfo"`
}

// ── UserSource — Estágio 2: Target Tracking ───────────────────────────────────

// UserSource monitoriza contas TikTok conhecidas via Evil0ctal.
// Delega os requests HTTP ao sidecar Python (sem TLS fingerprint preocupações).
type UserSource struct {
	sidecarURL string
	httpClient *req.Client
	dedup      *dedup.Deduplicator
	rdb        *redis.Client
	userAgent  string
	// Contas seed carregadas do config (complementadas pelo Redis em runtime)
	seedAccounts []string
}

// NewTikTokUserSource cria o coletor de Stage 2.
//   - sidecarURL: endereço do Evil0ctal (ex: http://localhost:8000)
//   - seedAccounts: usernames iniciais do config.yaml
//   - rdb: Redis client para ler alvos descobertos pelo Stage 1
//   - dedup: deduplicador
func NewTikTokUserSource(sidecarURL string, seedAccounts []string, rdb *redis.Client, dedup *dedup.Deduplicator) *UserSource {
	if sidecarURL == "" {
		sidecarURL = "http://localhost:8000"
	}
	sidecarURL = strings.TrimRight(sidecarURL, "/")

	client := req.C().ImpersonateChrome().SetTimeout(30 * time.Second)

	return &UserSource{
		sidecarURL:   sidecarURL,
		httpClient:   client,
		dedup:        dedup,
		rdb:          rdb,
		userAgent:    defaultUserAgent,
		seedAccounts: seedAccounts,
	}
}

func (u *UserSource) Name() string { return "TikTok-User-Tracker" }
func (u *UserSource) Close() error { return nil }

// Fetch carrega os alvos (seed + Redis) e para cada um extrai vídeos novos + links Discord da bio.
// O argumento query é ignorado — este source itera sobre a sua lista interna de alvos.
func (u *UserSource) Fetch(ctx context.Context, _ string) ([]DiscoveredVideo, error) {
	accounts := u.loadAccounts(ctx)
	if len(accounts) == 0 {
		log.Printf("[User-Tracker] Nenhum alvo configurado. Adicione contas em config.yaml ou aguarde o Estágio 1.")
		return nil, nil
	}

	var all []DiscoveredVideo
	for _, username := range accounts {
		videos, err := u.processAccount(ctx, username)
		if err != nil {
			log.Printf("[User-Tracker] erro ao processar @%s: %v", username, err)
			continue
		}
		all = append(all, videos...)
	}
	return all, nil
}

// loadAccounts combina as contas seed do config com os alvos descobertos pelo Stage 1 no Redis.
func (u *UserSource) loadAccounts(ctx context.Context) []string {
	seen := make(map[string]bool)
	var accounts []string

	for _, a := range u.seedAccounts {
		a = strings.TrimPrefix(a, "@")
		if a != "" && !seen[a] {
			seen[a] = true
			accounts = append(accounts, a)
		}
	}

	// Ler alvos do Redis (Stage 1 adiciona aqui)
	members, err := u.rdb.SMembers(ctx, RedisTargetsKey).Result()
	if err == nil {
		for _, m := range members {
			m = strings.TrimPrefix(m, "@")
			if m != "" && !seen[m] {
				seen[m] = true
				accounts = append(accounts, m)
			}
		}
	}

	log.Printf("[User-Tracker] %d conta(s) para monitorar", len(accounts))
	return accounts
}

// processAccount extrai vídeos novos e links Discord da bio de uma conta.
func (u *UserSource) processAccount(ctx context.Context, username string) ([]DiscoveredVideo, error) {
	// 1. Buscar perfil do utilizador
	profile, err := u.fetchUserProfile(ctx, username)
	if err != nil {
		return nil, err
	}

	user := profile.UserInfo.User
	log.Printf("[User-Tracker] @%s | bio: %q | bioLink: %q", username, user.Signature, user.BioLink.Link)

	var discovered []DiscoveredVideo

	// 2. Verificar links Discord diretos na bio
	discordLinks := sharedtiktok.ExtractDiscordLinks(user.Signature)
	if bioURL := user.BioLink.Link; bioURL != "" {
		// Tentar discord directo na bio URL
		discordLinks = append(discordLinks, sharedtiktok.ExtractDiscordLinks(bioURL)...)
		// Tentar raspar Linktree/Beacons se for um agregador
		if aggregators := sharedtiktok.ExtractAggregatorURLs(bioURL); len(aggregators) > 0 {
			for _, agg := range aggregators {
				discordLinks = append(discordLinks, ScrapeAggregator(ctx, &HTTPSource{httpClient: u.httpClient, userAgent: u.userAgent}, agg)...)
			}
		}
	}

	for _, link := range discordLinks {
		log.Printf("[User-Tracker] 🎯 Discord encontrado na bio de @%s: %s", username, link)
		// Publica como vídeo descoberto com ID especial "bio:{link}"
		bioID := "bio:" + username + ":" + link
		isProcessed, _ := u.dedup.CheckIfProcessed(ctx, "processed_job", bioID)
		if !isProcessed {
			discovered = append(discovered, DiscoveredVideo{
				ID:  bioID,
				URL: fmt.Sprintf("https://www.tiktok.com/@%s", username),
			})
		}
	}

	// 3. Buscar vídeos recentes e procurar links Discord nas descrições
	if user.SecUID != "" {
		videos, err := u.fetchUserPosts(ctx, user.SecUID, username)
		if err != nil {
			log.Printf("[User-Tracker] erro ao buscar posts de @%s: %v", username, err)
		} else {
			discovered = append(discovered, videos...)
		}
	}

	return discovered, nil
}

// fetchUserProfile chama o Evil0ctal para obter o perfil completo do utilizador.
func (u *UserSource) fetchUserProfile(ctx context.Context, username string) (*tiktokUserProfile, error) {
	endpoint := fmt.Sprintf("%s/api/tiktok/web/fetch_user_profile?uniqueId=%s", u.sidecarURL, username)
	resp, err := u.httpClient.R().SetContext(ctx).SetHeader("User-Agent", u.userAgent).Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("erro ao contactar Evil0ctal: %w", err)
	}

	body := resp.Bytes()

	var envelope userProfileResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("erro ao descodificar perfil: %w", err)
	}
	if envelope.Code != 200 {
		return nil, fmt.Errorf("Evil0ctal retornou código %d para o perfil de @%s", envelope.Code, username)
	}

	var profile tiktokUserProfile
	if err := json.Unmarshal(envelope.Data, &profile); err != nil {
		return nil, fmt.Errorf("erro ao descodificar data do perfil: %w", err)
	}

	return &profile, nil
}

// fetchUserPosts chama o Evil0ctal para obter os vídeos recentes de um utilizador.
func (u *UserSource) fetchUserPosts(ctx context.Context, secUID, username string) ([]DiscoveredVideo, error) {
	endpoint := fmt.Sprintf("%s/api/tiktok/web/fetch_user_post?secUid=%s&count=30&cursor=0", u.sidecarURL, secUID)
	resp, err := u.httpClient.R().SetContext(ctx).SetHeader("User-Agent", u.userAgent).Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar posts: %w", err)
	}

	body := resp.Bytes()

	var envelope struct {
		Code int `json:"code"`
		Data struct {
			ItemList []struct {
				ID   string `json:"id"`
				Desc string `json:"desc"`
			} `json:"itemList"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("erro ao descodificar posts: %w", err)
	}

	var discovered []DiscoveredVideo
	for _, item := range envelope.Data.ItemList {
		if item.ID == "" {
			continue
		}

		// Verificar links Discord na descrição
		discordLinks := sharedtiktok.ExtractDiscordLinks(item.Desc)
		if len(discordLinks) > 0 {
			log.Printf("[User-Tracker] 🎯 Discord na descrição do vídeo %s de @%s: %v", item.ID, username, discordLinks)
		}

		isProcessed, err := u.dedup.CheckIfProcessed(ctx, "processed_job", item.ID)
		if err != nil || isProcessed {
			continue
		}

		discovered = append(discovered, DiscoveredVideo{
			ID:  item.ID,
			URL: BuildTikTokURL(username, item.ID),
		})
	}

	if len(discovered) > 0 {
		log.Printf("[User-Tracker] @%s → %d vídeos novos", username, len(discovered))
	}

	return discovered, nil
}

// AddTarget adiciona um username ao Redis Set de alvos potenciais (chamado pelo Stage 1).
func AddTarget(ctx context.Context, rdb *redis.Client, username string) error {
	username = strings.TrimPrefix(username, "@")
	if username == "" {
		return nil
	}
	return rdb.SAdd(ctx, RedisTargetsKey, username).Err()
}

// ── Integrar Stage 1: guardar alvos descobertos ───────────────────────────────

// PushTargetIfNew guarda um username no Redis Set de alvos potenciais se ainda não existir.
// Chamado pelo Stage 1 quando detecta um utilizador que publicou vídeos com links Discord.
func PushTargetIfNew(ctx context.Context, rdb *redis.Client, username string) {
	if rdb == nil || username == "" {
		return
	}
	username = strings.TrimPrefix(username, "@")
	added, err := rdb.SAdd(ctx, RedisTargetsKey, username).Result()
	if err != nil {
		log.Printf("[Stage1→Stage2] erro ao guardar alvo @%s: %v", username, err)
		return
	}
	if added > 0 {
		log.Printf("[Stage1→Stage2] ✅ Novo alvo descoberto: @%s", username)
	}
}

// Garante que UserSource implementa a interface interna do pacote
var _ interface {
	Name() string
	Close() error
	Fetch(ctx context.Context, query string) ([]DiscoveredVideo, error)
} = (*UserSource)(nil)

