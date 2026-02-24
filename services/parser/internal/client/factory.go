package client

import (
	"log"

	"github.com/redis/go-redis/v9"
)

func NewDiscordClient(mode string, proxyURL string, token string, rdb *redis.Client) DiscordProvider {
	if mode == "api" {
		log.Println("[Factory] Inicializando Discord Client via API HTTP com OSINT Anonimo")
		if proxyURL != "" {
			log.Println("[Factory] 🌐 Proxy HTTP Configurado para contornar Rate Limits!")
		}
		return NewHTTPDiscordClient(proxyURL, rdb)
	}

	log.Println("[Factory] Inicializando Discord Client via Go-Rod (Browser Scraper)")
	return NewRodDiscordClient(token, rdb)
}
