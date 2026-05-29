package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestRepo creates an isolated in-memory SQLite repository for testing.
// Each test gets its own DB instance to avoid shared state.
func newTestRepo(t *testing.T) *SQLiteRepository {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:?_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)")
	if err != nil {
		t.Fatalf("falha ao abrir sqlite in-memory: %v", err)
	}
	db.SetMaxOpenConns(1)

	// Run migrations manually (same as sqlite_repo.go runMigrations)
	migration := `
	CREATE TABLE IF NOT EXISTS artifacts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_url TEXT NOT NULL,
		author_id TEXT NOT NULL,
		discord_invite_code TEXT NOT NULL,
		discord_server_name TEXT,
		discord_server_id TEXT,
		discord_member_count INTEGER,
		raw_ocr_text TEXT,
		risk_score INTEGER,
		processed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		discord_icon TEXT,
		discord_status TEXT,
		UNIQUE(source_url, discord_invite_code)
	);
	`
	if _, err := db.Exec(migration); err != nil {
		t.Fatalf("falha na migration: %v", err)
	}

	t.Cleanup(func() { db.Close() })

	return &SQLiteRepository{dbRead: db, dbWrite: db}
}

func TestSave(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	artifact := Artifact{
		SourceURL:         "https://tiktok.com/video/1",
		AuthorID:          "user1",
		DiscordInviteCode: "abc123",
		RawOcrText:        "join discord.gg/abc123",
		RiskScore:         0,
		DiscordStatus:     "pending",
	}

	id, err := repo.Save(context.Background(), artifact)
	if err != nil {
		t.Fatalf("Save() retornou erro: %v", err)
	}
	if id == "" {
		t.Error("Save() retornou ID vazio")
	}
}

func TestSave_Upsert(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	artifact := Artifact{
		SourceURL:         "https://tiktok.com/video/1",
		AuthorID:          "user1",
		DiscordInviteCode: "abc123",
		RawOcrText:        "primeiro texto",
		DiscordStatus:     "pending",
	}

	id1, err := repo.Save(context.Background(), artifact)
	if err != nil {
		t.Fatalf("primeiro Save() retornou erro: %v", err)
	}

	// Salva de novo com mesma source_url + invite_code (ON CONFLICT)
	artifact.RawOcrText = "segundo texto"
	id2, err := repo.Save(context.Background(), artifact)
	if err != nil {
		t.Fatalf("segundo Save() retornou erro: %v", err)
	}

	if id1 != id2 {
		t.Errorf("Upsert deveria retornar mesmo ID, got %s e %s", id1, id2)
	}

	// Verifica que o raw_ocr_text foi concatenado
	var rawText string
	err = repo.dbRead.QueryRow("SELECT raw_ocr_text FROM artifacts WHERE id = ?", id1).Scan(&rawText)
	if err != nil {
		t.Fatalf("erro lendo raw_ocr_text: %v", err)
	}

	if rawText != "primeiro texto | segundo texto" {
		t.Errorf("raw_ocr_text esperado concatenado, got %q", rawText)
	}
}

func TestUpdateEnrichedData(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	// Salva artifact pendente
	artifact := Artifact{
		SourceURL:         "https://tiktok.com/video/1",
		AuthorID:          "user1",
		DiscordInviteCode: "enrich-test",
		DiscordStatus:     "pending",
	}
	_, err := repo.Save(context.Background(), artifact)
	if err != nil {
		t.Fatalf("Save() retornou erro: %v", err)
	}

	// Atualiza com dados enriquecidos
	err = repo.UpdateEnrichedData(context.Background(), "enrich-test", "My Server", "123456", "https://cdn.discordapp.com/icons/123456/abc.png", 500, "active")
	if err != nil {
		t.Fatalf("UpdateEnrichedData() retornou erro: %v", err)
	}

	// Verifica os dados
	var serverName, serverID, icon, status string
	var memberCount int
	err = repo.dbRead.QueryRow(
		"SELECT discord_server_name, discord_server_id, discord_icon, discord_member_count, discord_status FROM artifacts WHERE discord_invite_code = ?",
		"enrich-test",
	).Scan(&serverName, &serverID, &icon, &memberCount, &status)
	if err != nil {
		t.Fatalf("erro lendo dados enriquecidos: %v", err)
	}

	if serverName != "My Server" {
		t.Errorf("serverName = %q, want %q", serverName, "My Server")
	}
	if serverID != "123456" {
		t.Errorf("serverID = %q, want %q", serverID, "123456")
	}
	if icon != "https://cdn.discordapp.com/icons/123456/abc.png" {
		t.Errorf("icon = %q, inesperado", icon)
	}
	if memberCount != 500 {
		t.Errorf("memberCount = %d, want 500", memberCount)
	}
	if status != "active" {
		t.Errorf("status = %q, want %q", status, "active")
	}
}

func TestUpdateStatus(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)

	artifact := Artifact{
		SourceURL:         "https://tiktok.com/video/1",
		AuthorID:          "user1",
		DiscordInviteCode: "status-test",
		DiscordStatus:     "pending",
	}
	_, err := repo.Save(context.Background(), artifact)
	if err != nil {
		t.Fatalf("Save() retornou erro: %v", err)
	}

	err = repo.UpdateStatus(context.Background(), "status-test", "rate_limited")
	if err != nil {
		t.Fatalf("UpdateStatus() retornou erro: %v", err)
	}

	var status string
	err = repo.dbRead.QueryRow("SELECT discord_status FROM artifacts WHERE discord_invite_code = ?", "status-test").Scan(&status)
	if err != nil {
		t.Fatalf("erro lendo status: %v", err)
	}
	if status != "rate_limited" {
		t.Errorf("status = %q, want %q", status, "rate_limited")
	}
}

func TestGetRateLimitedInvites(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	ctx := context.Background()

	// Salva artifacts com diferentes statuses
	artifacts := []Artifact{
		{SourceURL: "url1", AuthorID: "u1", DiscordInviteCode: "rl-1", DiscordStatus: "rate_limited"},
		{SourceURL: "url2", AuthorID: "u1", DiscordInviteCode: "rl-2", DiscordStatus: "rate_limited"},
		{SourceURL: "url3", AuthorID: "u1", DiscordInviteCode: "active-1", DiscordStatus: "active"},
		{SourceURL: "url4", AuthorID: "u1", DiscordInviteCode: "pending-1", DiscordStatus: "pending"},
		{SourceURL: "url5", AuthorID: "u1", DiscordInviteCode: "expired-1", DiscordStatus: "expired"},
	}

	for _, a := range artifacts {
		if _, err := repo.Save(ctx, a); err != nil {
			t.Fatalf("Save() erro para %s: %v", a.DiscordInviteCode, err)
		}
	}

	invites, err := repo.GetRateLimitedInvites(ctx, 50)
	if err != nil {
		t.Fatalf("GetRateLimitedInvites() erro: %v", err)
	}

	if len(invites) != 2 {
		t.Fatalf("esperava 2 invites rate_limited, got %d: %v", len(invites), invites)
	}

	// Verifica que ambos rate_limited estão no resultado
	found := make(map[string]bool)
	for _, code := range invites {
		found[code] = true
	}
	if !found["rl-1"] || !found["rl-2"] {
		t.Errorf("invites inesperados: %v, esperava rl-1 e rl-2", invites)
	}
}

func TestGetRateLimitedInvites_SkipsAlreadyEnriched(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	ctx := context.Background()

	// Salva artifact rate_limited
	a := Artifact{
		SourceURL:         "url1",
		AuthorID:          "u1",
		DiscordInviteCode: "enriched-rl",
		DiscordStatus:     "rate_limited",
	}
	if _, err := repo.Save(ctx, a); err != nil {
		t.Fatalf("Save() erro: %v", err)
	}

	// Enriquece (simula que foi parcialmente enriquecido mas status ficou rate_limited)
	err := repo.UpdateEnrichedData(ctx, "enriched-rl", "Server Real", "999", "icon.png", 100, "rate_limited")
	if err != nil {
		t.Fatalf("UpdateEnrichedData() erro: %v", err)
	}

	// GetRateLimitedInvites deve PULAR esse porque já tem server_name
	invites, err := repo.GetRateLimitedInvites(ctx, 50)
	if err != nil {
		t.Fatalf("GetRateLimitedInvites() erro: %v", err)
	}

	if len(invites) != 0 {
		t.Errorf("esperava 0 invites (já enriquecido), got %d: %v", len(invites), invites)
	}
}

func TestGetRateLimitedInvites_RespectsLimit(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	ctx := context.Background()

	// Cria 10 artifacts rate_limited
	for i := 0; i < 10; i++ {
		a := Artifact{
			SourceURL:         fmt.Sprintf("url-%d", i),
			AuthorID:          "u1",
			DiscordInviteCode: fmt.Sprintf("rl-%d", i),
			DiscordStatus:     "rate_limited",
		}
		if _, err := repo.Save(ctx, a); err != nil {
			t.Fatalf("Save() erro: %v", err)
		}
	}

	// Pede só 3
	invites, err := repo.GetRateLimitedInvites(ctx, 3)
	if err != nil {
		t.Fatalf("GetRateLimitedInvites() erro: %v", err)
	}

	if len(invites) != 3 {
		t.Errorf("esperava 3 invites (limit), got %d", len(invites))
	}
}

func TestGetRateLimitedInvites_Empty(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	ctx := context.Background()

	// Salva apenas artifacts com status != rate_limited
	a := Artifact{
		SourceURL:         "url1",
		AuthorID:          "u1",
		DiscordInviteCode: "active-1",
		DiscordStatus:     "active",
	}
	if _, err := repo.Save(ctx, a); err != nil {
		t.Fatalf("Save() erro: %v", err)
	}

	invites, err := repo.GetRateLimitedInvites(ctx, 50)
	if err != nil {
		t.Fatalf("GetRateLimitedInvites() erro: %v", err)
	}

	if invites != nil {
		t.Errorf("esperava nil para nenhum rate_limited, got %v", invites)
	}
}

func TestGetRateLimitedInvites_DeduplicatesInviteCodes(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	ctx := context.Background()

	// Mesmo invite_code de fontes diferentes (simula aparecer em múltiplos vídeos)
	for i := 0; i < 5; i++ {
		a := Artifact{
			SourceURL:         fmt.Sprintf("url-%d", i),
			AuthorID:          "u1",
			DiscordInviteCode: "same-code",
			DiscordStatus:     "rate_limited",
		}
		if _, err := repo.Save(ctx, a); err != nil {
			// ON CONFLICT vai impedir duplicados com mesmo source_url+invite_code,
			// mas com source_urls diferentes vai criar múltiplas rows
			t.Fatalf("Save() erro: %v", err)
		}
	}

	invites, err := repo.GetRateLimitedInvites(ctx, 50)
	if err != nil {
		t.Fatalf("GetRateLimitedInvites() erro: %v", err)
	}

	// DISTINCT deve retornar apenas 1
	if len(invites) != 1 {
		t.Errorf("esperava 1 invite (DISTINCT), got %d: %v", len(invites), invites)
	}
}
