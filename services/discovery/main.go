package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"discovery/internal/service"
	"discovery/internal/sources"

	"github.com/loviiin/project-argus/pkg/config"
	"github.com/loviiin/project-argus/pkg/dedup"
	"github.com/loviiin/project-argus/pkg/metrics"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
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

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	dedupSv := dedup.NewDeduplicator(rdb, cfg.Redis.TTLHours)

	log.Println("Inicializando driver do navegador (Discovery)...")
	tikTokSource := sources.NewTikTokRodSource(dedupSv)
	svc := service.NewDiscoveryService(js, rdb, []sources.Source{tikTokSource}, cfg.Discovery.Workers)

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

	discoveryMetrics := []metrics.MetricDef{
		{RedisKey: "argus:metrics:discovery:enqueued", PromName: "argus_discovery_enqueued_total", Help: "Total de videos enfileirados com sucesso", Type: "counter"},
		{RedisKey: "argus:metrics:discovery:duplicates", PromName: "argus_discovery_duplicates_total", Help: "Total de videos ignorados por duplicata", Type: "counter"},
		{RedisKey: "argus:metrics:discovery:failed", PromName: "argus_discovery_failed_total", Help: "Total de falhas criticas de processamento/publish", Type: "counter"},
	}
	go metrics.StartMetricsServer(":8081", rdb, discoveryMetrics)

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
