package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var ErrNoSessionsAvailable = errors.New("no active sessions available in the pool")

// TikTokSession representa a combinação obrigatória para evitar shadowban.
type TikTokSession struct {
	ID        string `json:"id"`
	Cookie    string `json:"cookie"`
	Proxy     string `json:"proxy"`
	UserAgent string `json:"user_agent"`
}

type Manager struct {
	rdb *redis.Client
}

func NewManager(rdb *redis.Client) *Manager {
	return &Manager{
		rdb: rdb,
	}
}

// AddSession insere uma nova sessão no pool com o TTL especificado (ex: 2 a 4 horas).
func (m *Manager) AddSession(ctx context.Context, cookie, proxy, userAgent string, ttl time.Duration) (string, error) {
	id := uuid.New().String()
	sess := TikTokSession{
		ID:        id,
		Cookie:    cookie,
		Proxy:     proxy,
		UserAgent: userAgent,
	}

	data, err := json.Marshal(sess)
	if err != nil {
		return "", fmt.Errorf("failed to marshal session: %w", err)
	}

	key := fmt.Sprintf("argus:session:data:%s", id)

	// Inicia transação do Redis (Pipeline) para consistência
	pipe := m.rdb.Pipeline()
	pipe.SetEx(ctx, key, data, ttl)
	pipe.SAdd(ctx, "argus:sessions:active", id)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to save session to redis: %w", err)
	}

	return id, nil
}

// GetRandomSession retorna uma sessão aleatória saudável do pool.
func (m *Manager) GetRandomSession(ctx context.Context) (*TikTokSession, error) {
	// Limita a 10 tentativas para encontrar uma sessão válida (não expirada)
	for i := 0; i < 10; i++ {
		// Pega um ID aleatório do Set
		id, err := m.rdb.SRandMember(ctx, "argus:sessions:active").Result()
		if err != nil {
			if err == redis.Nil {
				return nil, ErrNoSessionsAvailable
			}
			return nil, fmt.Errorf("failed to get random session id: %w", err)
		}

		key := fmt.Sprintf("argus:session:data:%s", id)
		data, err := m.rdb.Get(ctx, key).Result()
		if err != nil {
			if err == redis.Nil {
				// Chave expirou pelo TTL agressivo, mas ainda está no Set. Vamos limpar.
				m.rdb.SRem(ctx, "argus:sessions:active", id)
				continue
			}
			return nil, fmt.Errorf("failed to get session data: %w", err)
		}

		var sess TikTokSession
		if err := json.Unmarshal([]byte(data), &sess); err != nil {
			m.BurnSession(ctx, id) // Dados corrompidos, descarta
			continue
		}

		return &sess, nil
	}

	return nil, ErrNoSessionsAvailable
}

// BurnSession invalida a sessão para que não seja mais utilizada pelo pool.
func (m *Manager) BurnSession(ctx context.Context, id string) error {
	key := fmt.Sprintf("argus:session:data:%s", id)

	pipe := m.rdb.Pipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, "argus:sessions:active", id)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to burn session %s: %w", id, err)
	}

	return nil
}
