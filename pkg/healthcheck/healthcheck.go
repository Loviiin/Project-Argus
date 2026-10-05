package healthcheck

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

type Checker struct {
	NATS  *nats.Conn
	Redis *redis.Client
	DB    *sql.DB
}

func New(nc *nats.Conn, rdb *redis.Client, db *sql.DB) *Checker {
	return &Checker{
		NATS:  nc,
		Redis: rdb,
		DB:    db,
	}
}

func (c *Checker) Handler(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	details := make(map[string]string)

	if c.NATS != nil {
		if c.NATS.IsConnected() {
			details["nats"] = "ok"
		} else {
			status = "error"
			details["nats"] = "disconnected"
		}
	}

	if c.Redis != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := c.Redis.Ping(ctx).Err(); err != nil {
			status = "error"
			details["redis"] = "error: " + err.Error()
		} else {
			details["redis"] = "ok"
		}
	}

	if c.DB != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := c.DB.PingContext(ctx); err != nil {
			status = "error"
			details["db"] = "error: " + err.Error()
		} else {
			details["db"] = "ok"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if status == "error" {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  status,
		"details": details,
	})
}
