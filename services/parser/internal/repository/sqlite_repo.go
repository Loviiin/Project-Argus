package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	dbRead  *sql.DB
	dbWrite *sql.DB
}

func NewSQLiteRepository(path string) (*SQLiteRepository, error) {
	dbPath := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=synchronous(NORMAL)"

	dbWrite, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir sqlite write: %w", err)
	}
	dbWrite.SetMaxOpenConns(1)

	dbRead, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir sqlite read: %w", err)
	}
	dbRead.SetMaxOpenConns(10)

	repo := &SQLiteRepository{dbRead: dbRead, dbWrite: dbWrite}

	if err := repo.runMigrations(context.Background()); err != nil {
		return nil, fmt.Errorf("falha ao rodar migrations do sqlite: %w", err)
	}

	return repo, nil
}

var sqliteMigrations = []Migration{
	{
		Version: 1,
		Name:    "001_initial_schema",
		Query: `
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
		
			CREATE VIRTUAL TABLE IF NOT EXISTS artifacts_fts USING fts5(
				discord_invite_code,
				discord_server_name,
				raw_ocr_text,
				source_url,
				content='artifacts',
				content_rowid='id'
			);
		
			CREATE TRIGGER IF NOT EXISTS artifacts_ai AFTER INSERT ON artifacts BEGIN
				INSERT INTO artifacts_fts(rowid, discord_invite_code, discord_server_name, raw_ocr_text, source_url)
				VALUES (new.id, new.discord_invite_code, new.discord_server_name, new.raw_ocr_text, new.source_url);
			END;
		
			CREATE TRIGGER IF NOT EXISTS artifacts_ad AFTER DELETE ON artifacts BEGIN
				INSERT INTO artifacts_fts(artifacts_fts, rowid, discord_invite_code, discord_server_name, raw_ocr_text, source_url)
				VALUES ('delete', old.id, old.discord_invite_code, old.discord_server_name, old.raw_ocr_text, old.source_url);
			END;
		
			CREATE TRIGGER IF NOT EXISTS artifacts_au AFTER UPDATE ON artifacts BEGIN
				INSERT INTO artifacts_fts(artifacts_fts, rowid, discord_invite_code, discord_server_name, raw_ocr_text, source_url)
				VALUES ('delete', old.id, old.discord_invite_code, old.discord_server_name, old.raw_ocr_text, old.source_url);
				INSERT INTO artifacts_fts(rowid, discord_invite_code, discord_server_name, raw_ocr_text, source_url)
				VALUES (new.id, new.discord_invite_code, new.discord_server_name, new.raw_ocr_text, new.source_url);
			END;
		
			CREATE TABLE IF NOT EXISTS comments (
				cid TEXT PRIMARY KEY,
				aweme_id TEXT NOT NULL,
				text TEXT,
				digg_count INTEGER DEFAULT 0,
				reply_comment_total INTEGER DEFAULT 0,
				uid TEXT,
				nickname TEXT,
				unique_id TEXT,
				avatar_url TEXT,
				created_at DATETIME,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				reply_id TEXT
			);
		`,
	},
	{
		Version: 2,
		Name:    "002_add_unique_id",
		Query:   "ALTER TABLE comments ADD COLUMN unique_id TEXT;",
	},
	{
		Version: 3,
		Name:    "003_add_avatar_url",
		Query:   "ALTER TABLE comments ADD COLUMN avatar_url TEXT;",
	},
	{
		Version: 4,
		Name:    "004_add_tags",
		Query:   "ALTER TABLE artifacts ADD COLUMN tags TEXT DEFAULT '';",
	},
	{
		Version: 5,
		Name:    "005_add_nickname_index",
		Query:   "CREATE INDEX IF NOT EXISTS idx_comments_nickname ON comments(nickname);",
	},
	{
		Version: 6,
		Name:    "006_add_performance_indexes",
		Query: `
			CREATE INDEX IF NOT EXISTS idx_artifacts_discord_status ON artifacts(discord_status);
			CREATE INDEX IF NOT EXISTS idx_artifacts_discord_invite_code ON artifacts(discord_invite_code);
			CREATE INDEX IF NOT EXISTS idx_artifacts_discord_server_id ON artifacts(discord_server_id);
			CREATE INDEX IF NOT EXISTS idx_artifacts_processed_at ON artifacts(processed_at);
			CREATE INDEX IF NOT EXISTS idx_artifacts_member_count ON artifacts(discord_member_count);
			CREATE INDEX IF NOT EXISTS idx_comments_created_at ON comments(created_at);
			CREATE INDEX IF NOT EXISTS idx_comments_aweme_id ON comments(aweme_id);
		`,
	},
}

func (r *SQLiteRepository) runMigrations(ctx context.Context) error {
	slog.Info("Verificando schema do banco de dados (SQLite)...")

	_, err := r.dbWrite.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		slog.Error("Falha ao criar tabela schema_migrations no SQLite", "erro", err)
		return err
	}

	var count int
	err = r.dbWrite.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err == nil && count == 0 {
		var artifactsExists bool
		r.dbWrite.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name='artifacts')").Scan(&artifactsExists)
		if artifactsExists {
			slog.Info("Detectado banco existente (SQLite). Marcando migrations antigas como já aplicadas para não reexecutar.")
			// As migrations 1 a 5 já foram aplicadas manualmente antes da tabela schema_migrations existir
			for i := 0; i < 5; i++ {
				r.dbWrite.ExecContext(ctx, "INSERT INTO schema_migrations (version, name) VALUES (?, ?)", sqliteMigrations[i].Version, sqliteMigrations[i].Name)
			}
		}
	}

	for _, m := range sqliteMigrations {
		var applied bool
		err := r.dbWrite.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = ?)", m.Version).Scan(&applied)
		if err != nil {
			slog.Error("Falha ao verificar status da migration", "version", m.Version, "erro", err)
			continue
		}

		if applied {
			continue
		}

		slog.Info("Aplicando migration...", "version", m.Version, "name", m.Name)
		if _, err := r.dbWrite.ExecContext(ctx, m.Query); err != nil {
			slog.Warn("Aviso na migration (pode já estar aplicada parcialmente)", "name", m.Name, "erro", err)
		}

		if _, err := r.dbWrite.ExecContext(ctx, "INSERT INTO schema_migrations (version, name) VALUES (?, ?)", m.Version, m.Name); err != nil {
			slog.Error("Falha ao registrar migration aplicada", "name", m.Name, "erro", err)
		} else {
			slog.Info("Migration aplicada com sucesso.", "version", m.Version, "name", m.Name)
		}
	}

	// Migrations 1-5 podem ter sido marcadas como aplicadas sem rodar de fato (banco legado).
	// Garante que as colunas realmente existem, independente do schema_migrations.
	r.ensureColumns(ctx)

	slog.Info("Migrations concluídas no SQLite.")
	return nil
}

func (r *SQLiteRepository) ensureColumns(ctx context.Context) {
	required := []struct{ table, column, ddl string }{
		{"artifacts", "tags", "ALTER TABLE artifacts ADD COLUMN tags TEXT DEFAULT ''"},
		{"comments", "unique_id", "ALTER TABLE comments ADD COLUMN unique_id TEXT"},
		{"comments", "avatar_url", "ALTER TABLE comments ADD COLUMN avatar_url TEXT"},
	}
	for _, c := range required {
		var exists bool
		err := r.dbWrite.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM pragma_table_info(?) WHERE name = ?)", c.table, c.column).Scan(&exists)
		if err != nil {
			slog.Error("Falha ao verificar coluna", "table", c.table, "column", c.column, "erro", err)
			continue
		}
		if exists {
			continue
		}
		slog.Warn("Coluna ausente detectada, criando...", "table", c.table, "column", c.column)
		if _, err := r.dbWrite.ExecContext(ctx, c.ddl); err != nil {
			slog.Error("Falha ao criar coluna", "table", c.table, "column", c.column, "erro", err)
		}
	}
	if _, err := r.dbWrite.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_comments_nickname ON comments(nickname)"); err != nil {
		slog.Warn("Falha ao garantir idx_comments_nickname", "erro", err)
	}
}

func (r *SQLiteRepository) Save(ctx context.Context, a Artifact) (string, error) {
	query := `
		INSERT INTO artifacts 
		(source_url, author_id, discord_invite_code, discord_server_name, discord_server_id, discord_member_count, raw_ocr_text, risk_score, processed_at, discord_icon, discord_status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?)
		ON CONFLICT (source_url, discord_invite_code) DO UPDATE SET 
			raw_ocr_text = artifacts.raw_ocr_text || ' | ' || excluded.raw_ocr_text,
			processed_at = CURRENT_TIMESTAMP,
			discord_member_count = excluded.discord_member_count,
			discord_status = excluded.discord_status
		RETURNING id
	`
	var id string
	err := r.dbWrite.QueryRowContext(ctx, query,
		a.SourceURL,
		a.AuthorID,
		a.DiscordInviteCode,
		a.DiscordServerName,
		a.DiscordServerID,
		a.DiscordMemberCount,
		a.RawOcrText,
		a.RiskScore,
		a.DiscordIcon,
		a.DiscordStatus,
	).Scan(&id)
	return id, err
}

func (r *SQLiteRepository) UpdateEnrichedData(ctx context.Context, inviteCode, serverName, serverID, icon string, memberCount int, status string) error {
	query := `
		UPDATE artifacts 
		SET discord_server_name = ?,
		    discord_server_id = ?,
		    discord_icon = ?,
		    discord_member_count = ?,
			discord_status = ?,
			processed_at = CURRENT_TIMESTAMP
		WHERE discord_invite_code = ?
	`
	_, err := r.dbWrite.ExecContext(ctx, query, serverName, serverID, icon, memberCount, status, inviteCode)
	return err
}

func (r *SQLiteRepository) UpdateStatus(ctx context.Context, inviteCode, status string) error {
	query := `
		UPDATE artifacts 
		SET discord_status = ?,
		    processed_at = CURRENT_TIMESTAMP
		WHERE discord_invite_code = ?
	`
	_, err := r.dbWrite.ExecContext(ctx, query, status, inviteCode)
	return err
}

func (r *SQLiteRepository) UpdateTags(ctx context.Context, inviteCode, tags string) error {
	query := `
		UPDATE artifacts 
		SET tags = ?,
		    processed_at = CURRENT_TIMESTAMP
		WHERE discord_invite_code = ?
	`
	_, err := r.dbWrite.ExecContext(ctx, query, tags, inviteCode)
	return err
}

func (r *SQLiteRepository) Close(ctx context.Context) {
	r.dbRead.Close()
	r.dbWrite.Close()
}

func (r *SQLiteRepository) DB() *sql.DB {
	return r.dbRead
}

func (r *SQLiteRepository) GetRateLimitedInvites(ctx context.Context, limit int) ([]string, error) {
	query := `
		SELECT DISTINCT discord_invite_code 
		FROM artifacts 
		WHERE UPPER(discord_status) = 'RATE_LIMITED'
		  AND (discord_server_name IS NULL OR discord_server_name = '')
		LIMIT ?
	`
	rows, err := r.dbRead.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			continue
		}
		invites = append(invites, code)
	}
	return invites, nil
}
