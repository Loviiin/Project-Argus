package metrics

import (
	"context"
	"fmt"
	"log"
	"net/http"

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
				log.Printf("metrics: erro ao ler chave %s: %v", m.RedisKey, err)
				val = "0"
			}
			fmt.Fprintf(w, "# HELP %s %s\n", m.PromName, m.Help)
			fmt.Fprintf(w, "# TYPE %s %s\n", m.PromName, m.Type)
			fmt.Fprintf(w, "%s %s\n\n", m.PromName, val)
		}
	})

	log.Printf("Metrics server ouvindo em %s/metrics", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("metrics: falha ao iniciar servidor: %v", err)
	}
}
