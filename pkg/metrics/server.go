package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/redis/go-redis/v9"
)

// MetricDef define o mapeamento entre uma chave Redis e uma métrica Prometheus.
type MetricDef struct {
	RedisKey string
	PromName string
	Help     string
	Type     string // "counter" ou "gauge"
}

// StartMetricsServer inicia um servidor HTTP que expõe métricas no formato Prometheus.
func StartMetricsServer(port string, rdb *redis.Client, metricsDefs []MetricDef) {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		for _, m := range metricsDefs {
			val, err := rdb.Get(ctx, m.RedisKey).Result()
			if err == redis.Nil {
				val = "0"
			} else if err != nil {
				slog.Error("erro ao ler chave", "redis_key", m.RedisKey, "error", err)
				val = "0"
			}
			fmt.Fprintf(w, "# HELP %s %s\n", m.PromName, m.Help)
			fmt.Fprintf(w, "# TYPE %s %s\n", m.PromName, m.Type)
			fmt.Fprintf(w, "%s %s\n\n", m.PromName, val)
		}
	})

	slog.Info("Metrics server ouvindo", "port", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		slog.Error("falha ao iniciar servidor", "error", err)
		os.Exit(1)
	}
}
