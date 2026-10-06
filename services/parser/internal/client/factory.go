package client

import (
	"log"

	"github.com/redis/go-redis/v9"
)

func NewDiscordClient(proxyURL string, token string, fetchMode string, rdb *redis.Client) DiscordProvider {
	if fetchMode == "scraper" {
		log.Println("[Factory] Inicializando Discord Client via HTTP Scraper (OpenGraph)")
		if proxyURL != "" {
			log.Println("[Factory] 🌐 Proxy HTTP Configurado para contornar Rate Limits no Scraper!")
		}
		return NewScraperDiscordClient(proxyURL, rdb)
	} else if fetchMode == "hybrid" || fetchMode == "auto" || fetchMode == "" {
		log.Println("[Factory] Inicializando Discord Client HÍBRIDO (API -> Fallback Scraper)")
		if proxyURL != "" {
			log.Println("[Factory] 🌐 Proxy HTTP Configurado!")
		}
		apiCli := NewHTTPDiscordClient(proxyURL, rdb)
		scrapeCli := NewScraperDiscordClient(proxyURL, rdb)
		return NewHybridDiscordClient(apiCli, scrapeCli)
	}

	log.Println("[Factory] Inicializando Discord Client via API HTTP com OSINT Anonimo")
	if proxyURL != "" {
		log.Println("[Factory] 🌐 Proxy HTTP Configurado para contornar Rate Limits!")
	}
	return NewHTTPDiscordClient(proxyURL, rdb)
}
