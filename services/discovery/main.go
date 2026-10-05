package main

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"discovery/internal/service"
	"discovery/internal/sources"

	"github.com/loviiin/project-argus/pkg/config"
	"github.com/loviiin/project-argus/pkg/dedup"
	"github.com/loviiin/project-argus/pkg/healthcheck"
	"github.com/loviiin/project-argus/pkg/metrics"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg := config.LoadConfig()

	slog.Info("Argus Discovery Service (Publisher) iniciando...")

	// Inicia Pprof em background
	go func() {
		slog.Info("Iniciando Pprof do Discovery", "porta", ":6060")
		if err := http.ListenAndServe("127.0.0.1:6060", nil); err != nil {
			slog.Error("Pprof server falhou", "error", err)
		}
	}()

	nc, err := nats.Connect(cfg.Nats.URL)
	if err != nil {
		slog.Error("Erro NATS", "error", err)
		os.Exit(1)
	}
	js, err := nc.JetStream()
	if err != nil {
		slog.Error("Erro JetStream", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	// Garante que o stream argus-scraper existe
	if err := service.EnsureStream(js); err != nil {
		slog.Error("Erro criando stream argus-scraper", "error", err)
		os.Exit(1)
	}
	slog.Info("Stream argus-scraper (jobs.scrape) pronto")

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	dedupSv := dedup.NewDeduplicator(rdb, cfg.Redis.TTLHours)

	// ── Pipeline de Coleta em 2 Estágios ──────────────────────────────────────
	sidecarURL := cfg.TikTok.SidecarURL
	if v := os.Getenv("SIDECAR_URL"); v != "" {
		sidecarURL = v // override via env (Docker)
	}
	if sidecarURL == "" {
		sidecarURL = "http://localhost:8080"
	}

	// Estágio 1: Broad Discovery — busca por hashtag usando a API via Sidecar
	slog.Info("Inicializando Stage 1 (Hashtag Discovery via Sidecar API)")
	stage1 := sources.NewTikTokSignatureSearch(sidecarURL, cfg.TikTok.Ttwid, dedupSv)

	svc := service.NewDiscoveryService(js, rdb, []sources.Source{stage1}, cfg.Discovery.Workers)

	interval := time.Duration(cfg.Discovery.Interval) * time.Second
	if interval == 0 {
		interval = 30 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	cycle := func() {
		slog.Info("--- Iniciando ciclo de discovery ---")
		svc.Run(cfg.Discovery.Hashtags)
	}

	go cycle()

	discoveryMetrics := []metrics.MetricDef{
		{RedisKey: "argus:metrics:discovery:enqueued", PromName: "argus_discovery_enqueued_total", Help: "Total de videos enfileirados com sucesso", Type: "counter"},
		{RedisKey: "argus:metrics:discovery:duplicates", PromName: "argus_discovery_duplicates_total", Help: "Total de videos ignorados por duplicata", Type: "counter"},
		{RedisKey: "argus:metrics:discovery:failed", PromName: "argus_discovery_failed_total", Help: "Total de falhas criticas de processamento/publish", Type: "counter"},
	}
	healthHandler := healthcheck.New(nc, rdb, nil).Handler
	go metrics.StartMetricsServer(":8081", rdb, discoveryMetrics, healthHandler)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("Discovery Service (Publisher) rodando! Publicando em jobs.scrape...")

	for {
		select {
		case <-ticker.C:
			cycle()
		case <-sig:
			slog.Info("Encerrando Discovery Service...")
			svc.Close()
			return
		}
	}
}
