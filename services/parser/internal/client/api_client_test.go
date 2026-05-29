package client

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// NewHTTPDiscordClient – constructor behavior
// ---------------------------------------------------------------------------

func TestNewHTTPDiscordClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		proxyURL     string
		wantProxy    bool
		wantTimeout  time.Duration
		wantNilHTTP  bool
	}{
		{
			name:        "without proxy",
			proxyURL:    "",
			wantProxy:   false,
			wantTimeout: 15 * time.Second,
		},
		{
			name:        "with valid HTTP proxy",
			proxyURL:    "http://proxy.example.com:8080",
			wantProxy:   true,
			wantTimeout: 15 * time.Second,
		},
		{
			name:        "with valid SOCKS5 proxy",
			proxyURL:    "socks5://user:pass@proxy.example.com:1080",
			wantProxy:   true,
			wantTimeout: 15 * time.Second,
		},
		{
			name:        "with invalid proxy URL falls back to no proxy",
			proxyURL:    "://totally-broken",
			wantProxy:   false,
			wantTimeout: 15 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := NewHTTPDiscordClient(tt.proxyURL, nil)

			if client == nil {
				t.Fatal("expected non-nil client")
			}
			if client.httpClient == nil {
				t.Fatal("expected non-nil httpClient")
			}
			if client.proxyEnabled != tt.wantProxy {
				t.Errorf("proxyEnabled = %v, want %v", client.proxyEnabled, tt.wantProxy)
			}
			if client.httpClient.Timeout != tt.wantTimeout {
				t.Errorf("timeout = %v, want %v", client.httpClient.Timeout, tt.wantTimeout)
			}

			transport, ok := client.httpClient.Transport.(*http.Transport)
			if !ok {
				t.Fatal("expected *http.Transport as underlying transport")
			}

			if tt.wantProxy {
				if transport.Proxy == nil {
					t.Error("expected transport.Proxy to be set when proxy is enabled")
				}
			} else {
				if transport.Proxy != nil {
					t.Error("expected transport.Proxy to be nil when proxy is disabled")
				}
			}
		})
	}
}

func TestNewHTTPDiscordClient_RedisFieldIsStored(t *testing.T) {
	t.Parallel()

	// Pass nil redis – the constructor should still succeed and store it.
	client := NewHTTPDiscordClient("", nil)
	if client.rdb != nil {
		t.Error("expected rdb to be nil when nil is provided")
	}
}

// ---------------------------------------------------------------------------
// DiscordInviteResponse – JSON deserialization
// ---------------------------------------------------------------------------

func TestDiscordInviteResponse_JSONDeserialization(t *testing.T) {
	t.Parallel()

	expiresAt := "2026-06-01T00:00:00+00:00"

	tests := []struct {
		name       string
		payload    string
		wantErr    bool
		validate   func(t *testing.T, r *DiscordInviteResponse)
	}{
		{
			name: "full payload with all fields",
			payload: `{
				"code": "abc123",
				"guild": {
					"id": "111222333",
					"name": "Test Guild",
					"icon": "icon_hash_abc"
				},
				"approximate_member_count": 42000,
				"expires_at": "2026-06-01T00:00:00+00:00"
			}`,
			validate: func(t *testing.T, r *DiscordInviteResponse) {
				t.Helper()
				if r.Code != "abc123" {
					t.Errorf("Code = %q, want %q", r.Code, "abc123")
				}
				if r.Guild.ID != "111222333" {
					t.Errorf("Guild.ID = %q, want %q", r.Guild.ID, "111222333")
				}
				if r.Guild.Name != "Test Guild" {
					t.Errorf("Guild.Name = %q, want %q", r.Guild.Name, "Test Guild")
				}
				if r.Guild.Icon != "icon_hash_abc" {
					t.Errorf("Guild.Icon = %q, want %q", r.Guild.Icon, "icon_hash_abc")
				}
				if r.ApproximateMemberCount != 42000 {
					t.Errorf("ApproximateMemberCount = %d, want %d", r.ApproximateMemberCount, 42000)
				}
				if r.ExpiresAt == nil {
					t.Fatal("ExpiresAt should not be nil")
				}
				if *r.ExpiresAt != expiresAt {
					t.Errorf("ExpiresAt = %q, want %q", *r.ExpiresAt, expiresAt)
				}
			},
		},
		{
			name: "null expires_at results in nil pointer",
			payload: `{
				"code": "xyz",
				"guild": {"id": "1", "name": "G", "icon": ""},
				"approximate_member_count": 10,
				"expires_at": null
			}`,
			validate: func(t *testing.T, r *DiscordInviteResponse) {
				t.Helper()
				if r.ExpiresAt != nil {
					t.Errorf("ExpiresAt = %v, want nil", *r.ExpiresAt)
				}
				if r.Code != "xyz" {
					t.Errorf("Code = %q, want %q", r.Code, "xyz")
				}
			},
		},
		{
			name: "missing optional fields use zero values",
			payload: `{
				"code": "minimal"
			}`,
			validate: func(t *testing.T, r *DiscordInviteResponse) {
				t.Helper()
				if r.Code != "minimal" {
					t.Errorf("Code = %q, want %q", r.Code, "minimal")
				}
				if r.Guild.ID != "" {
					t.Errorf("Guild.ID = %q, want empty string", r.Guild.ID)
				}
				if r.Guild.Name != "" {
					t.Errorf("Guild.Name = %q, want empty string", r.Guild.Name)
				}
				if r.ApproximateMemberCount != 0 {
					t.Errorf("ApproximateMemberCount = %d, want 0", r.ApproximateMemberCount)
				}
				if r.ExpiresAt != nil {
					t.Errorf("ExpiresAt should be nil for missing field, got %v", *r.ExpiresAt)
				}
			},
		},
		{
			name:    "empty JSON object",
			payload: `{}`,
			validate: func(t *testing.T, r *DiscordInviteResponse) {
				t.Helper()
				if r.Code != "" {
					t.Errorf("Code = %q, want empty string", r.Code)
				}
				if r.ApproximateMemberCount != 0 {
					t.Errorf("ApproximateMemberCount = %d, want 0", r.ApproximateMemberCount)
				}
			},
		},
		{
			name:    "extra unknown fields are ignored",
			payload: `{"code":"inv","unknown_field":"ignored","guild":{"id":"9","name":"N","icon":"I","extra":true},"approximate_member_count":5}`,
			validate: func(t *testing.T, r *DiscordInviteResponse) {
				t.Helper()
				if r.Code != "inv" {
					t.Errorf("Code = %q, want %q", r.Code, "inv")
				}
				if r.Guild.ID != "9" {
					t.Errorf("Guild.ID = %q, want %q", r.Guild.ID, "9")
				}
				if r.ApproximateMemberCount != 5 {
					t.Errorf("ApproximateMemberCount = %d, want %d", r.ApproximateMemberCount, 5)
				}
			},
		},
		{
			name:    "malformed JSON returns error",
			payload: `{not valid json`,
			wantErr: true,
		},
		{
			name: "member count as zero is valid",
			payload: `{
				"code": "zero",
				"guild": {"id": "0", "name": "", "icon": ""},
				"approximate_member_count": 0
			}`,
			validate: func(t *testing.T, r *DiscordInviteResponse) {
				t.Helper()
				if r.ApproximateMemberCount != 0 {
					t.Errorf("ApproximateMemberCount = %d, want 0", r.ApproximateMemberCount)
				}
			},
		},
		{
			name: "large member count",
			payload: `{
				"code": "big",
				"guild": {"id": "1", "name": "Big Server", "icon": ""},
				"approximate_member_count": 1500000
			}`,
			validate: func(t *testing.T, r *DiscordInviteResponse) {
				t.Helper()
				if r.ApproximateMemberCount != 1_500_000 {
					t.Errorf("ApproximateMemberCount = %d, want 1500000", r.ApproximateMemberCount)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var resp DiscordInviteResponse
			err := json.Unmarshal([]byte(tt.payload), &resp)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, &resp)
			}
		})
	}
}

func TestDiscordInviteResponse_RoundTrip(t *testing.T) {
	t.Parallel()

	expiresAt := "2026-12-31T23:59:59Z"
	original := DiscordInviteResponse{
		Code:                   "roundtrip",
		ApproximateMemberCount: 999,
		ExpiresAt:              &expiresAt,
	}
	original.Guild.ID = "555"
	original.Guild.Name = "RoundTrip Guild"
	original.Guild.Icon = "rt_icon"

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded DiscordInviteResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Code != original.Code {
		t.Errorf("Code = %q, want %q", decoded.Code, original.Code)
	}
	if decoded.Guild.ID != original.Guild.ID {
		t.Errorf("Guild.ID = %q, want %q", decoded.Guild.ID, original.Guild.ID)
	}
	if decoded.Guild.Name != original.Guild.Name {
		t.Errorf("Guild.Name = %q, want %q", decoded.Guild.Name, original.Guild.Name)
	}
	if decoded.Guild.Icon != original.Guild.Icon {
		t.Errorf("Guild.Icon = %q, want %q", decoded.Guild.Icon, original.Guild.Icon)
	}
	if decoded.ApproximateMemberCount != original.ApproximateMemberCount {
		t.Errorf("ApproximateMemberCount = %d, want %d", decoded.ApproximateMemberCount, original.ApproximateMemberCount)
	}
	if decoded.ExpiresAt == nil || *decoded.ExpiresAt != *original.ExpiresAt {
		t.Errorf("ExpiresAt mismatch")
	}
}

// ---------------------------------------------------------------------------
// ErrCircuitOpen – sentinel error
// ---------------------------------------------------------------------------

func TestErrCircuitOpen(t *testing.T) {
	t.Parallel()

	if ErrCircuitOpen == nil {
		t.Fatal("ErrCircuitOpen should not be nil")
	}
	if ErrCircuitOpen.Error() != "circuit breaker open" {
		t.Errorf("ErrCircuitOpen.Error() = %q, want %q", ErrCircuitOpen.Error(), "circuit breaker open")
	}
}

// ---------------------------------------------------------------------------
// NewDiscordClient factory – returns the correct concrete type
// ---------------------------------------------------------------------------

func TestNewDiscordClient_ReturnsHTTPDiscordClient(t *testing.T) {
	t.Parallel()

	provider := NewDiscordClient("", "", nil)
	if provider == nil {
		t.Fatal("expected non-nil DiscordProvider")
	}

	concrete, ok := provider.(*HTTPDiscordClient)
	if !ok {
		t.Fatalf("expected *HTTPDiscordClient, got %T", provider)
	}
	if concrete.proxyEnabled {
		t.Error("expected proxyEnabled=false when no proxy URL is provided")
	}
}

func TestNewDiscordClient_WithProxy(t *testing.T) {
	t.Parallel()

	provider := NewDiscordClient("http://proxy.local:3128", "", nil)
	concrete, ok := provider.(*HTTPDiscordClient)
	if !ok {
		t.Fatalf("expected *HTTPDiscordClient, got %T", provider)
	}
	if !concrete.proxyEnabled {
		t.Error("expected proxyEnabled=true when a valid proxy URL is provided")
	}
}
