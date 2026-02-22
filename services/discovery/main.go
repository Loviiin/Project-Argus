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

	fmt.Println("Argus Discovery Service (Publisher) iniciando...")

	nc, err := nats.Connect(cfg.Nats.URL)
	if err != nil {
		log.Fatal("Erro NATS:", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		log.Fatal("Erro JetStream:", err)
	}
	defer nc.Close()

	// Garante que o stream SCRAPE existe
	if err := service.EnsureStream(js); err != nil {
		log.Fatal("Erro criando stream SCRAPE:", err)
	}
	log.Println("Stream SCRAPE (jobs.scrape) pronto")

	dedup := repository.NewDeduplicator(cfg.Redis.Address, cfg.Redis.Password, cfg.Redis.DB)
	log.Println("Inicializando driver do navegador (Discovery)...")
	tikTokSource := sources.NewTikTokRodSource(dedup)

	svc := service.NewDiscoveryService(js, []sources.Source{tikTokSource}, cfg.Discovery.Workers)

	interval := time.Duration(cfg.Discovery.Interval) * time.Second
	if interval == 0 {
		interval = 30 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	cycle := func() {
		fmt.Println("\n--- Iniciando ciclo de discovery ---")
		svc.Run(cfg.Discovery.Hashtags)
	}

	go cycle()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Discovery Service (Publisher) rodando! Publicando em jobs.scrape...")

	for {
		select {
		case <-ticker.C:
			cycle()
		case <-sig:
			fmt.Println("\nEncerrando Discovery Service...")
			svc.Close()
			return
		}
	}
}
