package client

import (
	"log"

	"github.com/redis/go-redis/v9"
)

func NewDiscordClient(proxyURL string, token string, rdb *redis.Client) DiscordProvider {
	log.Println("[Factory] Inicializando Discord Client via API HTTP com OSINT Anonimo")
	if proxyURL != "" {
		log.Println("[Factory] 🌐 Proxy HTTP Configurado para contornar Rate Limits!")
	}
	return NewHTTPDiscordClient(proxyURL, rdb)
}
