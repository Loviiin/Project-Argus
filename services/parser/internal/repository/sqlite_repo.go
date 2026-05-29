package repository

import (
	"context"
	"database/sql"
	"fmt"

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

func (r *SQLiteRepository) runMigrations(ctx context.Context) error {
	query := `
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

	-- Virtual table para FTS5
	CREATE VIRTUAL TABLE IF NOT EXISTS artifacts_fts USING fts5(
		discord_invite_code,
		discord_server_name,
		raw_ocr_text,
		source_url,
		content='artifacts',
		content_rowid='id'
	);

	-- Triggers para manter a tabela FTS atualizada
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

	-- Adicionar colunas caso a tabela já exista
	ALTER TABLE comments ADD COLUMN unique_id TEXT;
	ALTER TABLE comments ADD COLUMN avatar_url TEXT;
	`
	// Ignoramos o erro do ALTER TABLE pois se as colunas já existirem ele falha graciosamente
	r.dbWrite.ExecContext(ctx, query)
	
	// Adicionar índices para otimizar queries
	r.dbWrite.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_comments_nickname ON comments(nickname);")
	return nil
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
