package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"discovery/internal/repository"
	"discovery/internal/service"
	"discovery/internal/sources"

	"github.com/loviiin/project-argus/pkg/config"
	"github.com/nats-io/nats.go"
)

func main() {
	cfg := config.LoadConfig()

	fmt.Println("Argus Discovery Service (Go-Rod) iniciando...")

	nc, err := nats.Connect(cfg.Nats.URL)
	if err != nil {
		log.Fatal("Erro NATS:", err)
	}
	js, _ := nc.JetStream()
	defer nc.Close()

	dedup := repository.NewDeduplicator(cfg.Redis.Address, cfg.Redis.Password, cfg.Redis.DB)
	log.Println("Inicializando driver do navegador...")
	tikTokSource := sources.NewTikTokRodSource()

	svc := service.NewDiscoveryService(dedup, js, []sources.Source{tikTokSource}, cfg.Discovery.Workers)

	interval := time.Duration(cfg.Discovery.Interval) * time.Second
	if interval == 0 {
		interval = 30 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	cycle := func() {
		fmt.Println("\n--- Iniciando ciclo de busca ---")
		svc.Run(cfg.Discovery.Hashtags)
	}

	go cycle()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Discovery Service rodando! Aguardando jobs...")

	for {
		select {
		case <-ticker.C:
			cycle()
		case <-sig:
			fmt.Println("Encerrando...")
			return
		}
	}
}
