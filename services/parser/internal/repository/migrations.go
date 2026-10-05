package repository

import (
	"context"
	"log/slog"
)

type Migration struct {
	Version int
	Name    string
	Query   string
}

var postgresMigrations = []Migration{
	{
		Version: 1,
		Name:    "001_initial_schema",
		Query: `CREATE TABLE IF NOT EXISTS artifacts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_platform VARCHAR(50) DEFAULT 'tiktok',
			source_url TEXT NOT NULL, 
			author_id VARCHAR(100),
			discord_invite_code VARCHAR(50),
			discord_server_name VARCHAR(255),
			discord_server_id VARCHAR(100),
			discord_member_count INT,
			raw_ocr_text TEXT, 
			risk_score INT DEFAULT 0,
			processed_at TIMESTAMP DEFAULT NOW(),
			UNIQUE(source_url, discord_invite_code)
		);`,
	},
	{
		Version: 2,
		Name:    "002_add_discord_icon",
		Query:   "ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS discord_icon TEXT;",
	},
	{
		Version: 3,
		Name:    "003_ensure_unique_constraint",
		Query:   "CREATE UNIQUE INDEX IF NOT EXISTS artifacts_source_url_discord_invite_code_key ON artifacts (source_url, discord_invite_code);",
	},
	{
		Version: 4,
		Name:    "004_add_discord_status",
		Query:   "ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS discord_status VARCHAR(50) DEFAULT 'pending';",
	},
	{
		Version: 5,
		Name:    "005_create_comments_table",
		Query: `CREATE TABLE IF NOT EXISTS comments (
			cid VARCHAR(100) PRIMARY KEY,
			aweme_id VARCHAR(100) NOT NULL,
			text TEXT,
			digg_count INT DEFAULT 0,
			reply_comment_total INT DEFAULT 0,
			uid VARCHAR(100),
			nickname VARCHAR(255),
			created_at TIMESTAMP,
			updated_at TIMESTAMP DEFAULT NOW()
		);`,
	},
	{
		Version: 6,
		Name:    "006_add_reply_id_to_comments",
		Query:   "ALTER TABLE comments ADD COLUMN IF NOT EXISTS reply_id VARCHAR(100);",
	},
	{
		Version: 7,
		Name:    "007_add_performance_indexes",
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

func (r *PostgresRepository) runMigrations() error {
	slog.Info("Verificando schema do banco de dados (Postgres)...")

	_, err := r.db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			applied_at TIMESTAMP DEFAULT NOW()
		);
	`)
	if err != nil {
		slog.Error("Falha ao criar tabela schema_migrations no Postgres", "erro", err)
		return err
	}

	var count int
	err = r.db.QueryRow(context.Background(), "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err == nil && count == 0 {
		var artifactsExists bool
		r.db.QueryRow(context.Background(), "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'artifacts')").Scan(&artifactsExists)
		if artifactsExists {
			slog.Info("Detectado banco existente (Postgres). Marcando migrations antigas como já aplicadas para não reexecutar.")
			// As migrations 1 a 6 já foram aplicadas manualmente antes da tabela schema_migrations existir
			for i := 0; i < 6; i++ {
				r.db.Exec(context.Background(), "INSERT INTO schema_migrations (version, name) VALUES ($1, $2)", postgresMigrations[i].Version, postgresMigrations[i].Name)
			}
		}
	}

	for _, m := range postgresMigrations {
		var applied bool
		err := r.db.QueryRow(context.Background(), "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", m.Version).Scan(&applied)
		if err != nil {
			slog.Error("Falha ao verificar status da migration", "version", m.Version, "erro", err)
			continue
		}

		if applied {
			continue
		}

		slog.Info("Aplicando migration...", "version", m.Version, "name", m.Name)
		if _, err := r.db.Exec(context.Background(), m.Query); err != nil {
			slog.Warn("Aviso na migration (pode já estar aplicada parcialmente)", "name", m.Name, "erro", err)
		}

		if _, err := r.db.Exec(context.Background(), "INSERT INTO schema_migrations (version, name) VALUES ($1, $2)", m.Version, m.Name); err != nil {
			slog.Error("Falha ao registrar migration aplicada", "name", m.Name, "erro", err)
		} else {
			slog.Info("Migration aplicada com sucesso.", "version", m.Version, "name", m.Name)
		}
	}

	slog.Info("Migrations concluídas no Postgres.")
	return nil
}
