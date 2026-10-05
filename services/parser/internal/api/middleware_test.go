package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func doRequest(h http.Handler, remoteAddr string) int {
	req := httptest.NewRequest(http.MethodGet, "/api/artifacts", nil)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestRateLimiter_Returns429AfterBurst(t *testing.T) {
	const burst = 5
	rl := NewIPRateLimiter(0.0001, burst) // refill praticamente nulo durante o teste
	h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < burst; i++ {
		if code := doRequest(h, "10.0.0.1:1234"); code != http.StatusOK {
			t.Fatalf("request %d: esperava 200, got %d", i+1, code)
		}
	}
	if code := doRequest(h, "10.0.0.1:1234"); code != http.StatusTooManyRequests {
		t.Fatalf("esperava 429 após burst, got %d", code)
	}

	// Outro IP não deve ser afetado
	if code := doRequest(h, "10.0.0.2:5678"); code != http.StatusOK {
		t.Fatalf("IP diferente deveria passar, got %d", code)
	}
}

func TestRateLimiter_CleanupRemovesIdleVisitors(t *testing.T) {
	rl := NewIPRateLimiter(10, 10)
	current := time.Now()
	rl.now = func() time.Time { return current }

	rl.getLimiter("10.0.0.1")
	current = current.Add(visitorTTL + time.Second)
	rl.cleanup()

	rl.mu.Lock()
	defer rl.mu.Unlock()
	if len(rl.visitors) != 0 {
		t.Fatalf("esperava map vazio após cleanup, got %d", len(rl.visitors))
	}
}

func TestParseIntWithBounds_ZeroLimitUsesFallback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/artifacts?limit=0", nil)
	if got := parseIntWithBounds(req, "limit", 50, 200); got != 50 {
		t.Fatalf("limit=0 deveria cair no fallback 50, got %d", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/artifacts?limit=9999", nil)
	if got := parseIntWithBounds(req, "limit", 50, 200); got != 200 {
		t.Fatalf("limit=9999 deveria ser limitado a 200, got %d", got)
	}
}
