package repository

import (
	"context"
	"log"
)

func (r *ArtifactRepository) runMigrations() error {
	log.Println("Verificando schema do banco de dados...")

	queries := []struct {
		name  string
		query string
	}{
		{
			name: "001_initial_schema",
			query: `CREATE TABLE IF NOT EXISTS artifacts (
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
			name:  "002_add_discord_icon",
			query: "ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS discord_icon TEXT;",
		},
	}

	for _, m := range queries {
		if _, err := r.db.Exec(context.Background(), m.query); err != nil {
			log.Printf("Erro na migration [%s]: %v", m.name, err)
			// Não retornamos erro fatal aqui para tentar rodar as próximas caso seja um erro "trivial"
			// (embora em prod devêssemos ser mais estritos)
		} else {
			// log.Printf("Migration [%s] verificada.", m.name)
		}
	}

	log.Println("Migrations concluídas.")
	return nil
}
