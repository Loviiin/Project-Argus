package api

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultRateLimitRPS   = 10.0
	defaultRateLimitBurst = 30
	visitorTTL            = 3 * time.Minute
	cleanupInterval       = time.Minute
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimiter aplica um token bucket independente por IP de origem.
type IPRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rps      rate.Limit
	burst    int
	now      func() time.Time
}

// NewIPRateLimiter cria o limiter e inicia a goroutine que remove IPs inativos
// (evita crescimento ilimitado do map).
func NewIPRateLimiter(rps float64, burst int) *IPRateLimiter {
	rl := &IPRateLimiter{
		visitors: make(map[string]*visitor),
		rps:      rate.Limit(rps),
		burst:    burst,
		now:      time.Now,
	}
	go rl.cleanupLoop()
	return rl
}

// NewIPRateLimiterFromEnv lê RATE_LIMIT_RPS e RATE_LIMIT_BURST (com defaults seguros).
func NewIPRateLimiterFromEnv() *IPRateLimiter {
	rps := defaultRateLimitRPS
	if v, err := strconv.ParseFloat(os.Getenv("RATE_LIMIT_RPS"), 64); err == nil && v > 0 {
		rps = v
	}
	burst := defaultRateLimitBurst
	if v, err := strconv.Atoi(os.Getenv("RATE_LIMIT_BURST")); err == nil && v > 0 {
		burst = v
	}
	slog.Info("Rate limiter configurado", "rps", rps, "burst", burst)
	return NewIPRateLimiter(rps, burst)
}

func (rl *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, ok := rl.visitors[ip]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(rl.rps, rl.burst)}
		rl.visitors[ip] = v
	}
	v.lastSeen = rl.now()
	return v.limiter
}

func (rl *IPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		rl.cleanup()
	}
}

func (rl *IPRateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := rl.now().Add(-visitorTTL)
	for ip, v := range rl.visitors {
		if v.lastSeen.Before(cutoff) {
			delete(rl.visitors, ip)
		}
	}
}

// Middleware retorna 429 quando o IP excede o limite.
func (rl *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.getLimiter(clientIP(r)).Allow() {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "too many requests"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP usa RemoteAddr de propósito: X-Forwarded-For pode ser forjado pelo
// cliente para burlar o limite. Se for colocado atrás de um proxy reverso,
// ajustar aqui para confiar apenas no header do proxy.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
