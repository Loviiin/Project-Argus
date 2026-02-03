package main

import (
	"discovery/internal/repository"
	"discovery/internal/service"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/loviiin/project-argus/pkg/config"
	"github.com/nats-io/nats.go"
)

func main() {
	// 1. Carrega Configuração Unificada
	cfg := config.LoadConfig()

	fmt.Println("Argus Discovery Service Iniciando...")

	// 2. Conecta Infra (NATS + Redis) usando Config
	nc, err := nats.Connect(cfg.Nats.URL)
	if err != nil {
		log.Fatal("Erro NATS:", err)
	}
	js, _ := nc.JetStream()
	defer nc.Close()

	dedup := repository.NewDeduplicator(cfg.Redis.Address, cfg.Redis.Password, cfg.Redis.DB)
	defer dedup.Close()

	// 3. Inicializa Serviço
	svc := service.NewDiscoveryService(dedup, js)

	// 4. Loop de Execução
	interval := time.Duration(cfg.Discovery.Interval) * time.Second
	if interval == 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	cycle := func() {
		fmt.Println("--- Iniciando ciclo de busca ---")
		svc.Run(cfg.Discovery.Hashtags)
	}

	go cycle()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Discovery Service rodando! (Ctrl+C para sair)")

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
