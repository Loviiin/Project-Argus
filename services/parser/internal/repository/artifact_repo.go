package repository

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
)

type Artifact struct {
	SourceURL          string
	AuthorID           string
	DiscordInviteCode  string
	DiscordServerName  string
	DiscordServerID    string
	DiscordMemberCount int
	RawOcrText         string
	RiskScore          int
}

type ArtifactRepository struct {
	db *pgx.Conn
}

func NewArtifactRepository(databaseURL string) (*ArtifactRepository, error) {
	conn, err := pgx.Connect(context.Background(), databaseURL)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar no postgres: %w", err)
	}

	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("banco não responde: %w", err)
	}

	repo := &ArtifactRepository{db: conn}

	if err := repo.initSchema(); err != nil {
		conn.Close(context.Background())
		return nil, fmt.Errorf("falha ao criar tabela: %w", err)
	}

	return repo, nil
}

func (r *ArtifactRepository) initSchema() error {
	query := `
        CREATE TABLE IF NOT EXISTS artifacts (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        
            -- Origem
            source_platform VARCHAR(50) DEFAULT 'tiktok',
            source_url TEXT NOT NULL, 
            author_id VARCHAR(100), -- @usuario do tiktok
        
            -- Inteligência Extraída
            discord_invite_code VARCHAR(50),
            discord_server_name VARCHAR(255),
            discord_server_id VARCHAR(100),
            discord_member_count INT,
        
            -- Auditoria
            raw_ocr_text TEXT, 
            risk_score INT DEFAULT 0,
            processed_at TIMESTAMP DEFAULT NOW(),

            -- Garante que o mesmo convite na mesma imagem não seja duplicado,
            -- mas permite múltiplos convites diferentes para a mesma imagem.
            UNIQUE(source_url, discord_invite_code)
        );

        -- Criação de Índices (Idempotente: IF NOT EXISTS ajuda, mas em SQL puro
        -- o create index dá erro se já existe. Vamos ignorar erro de índice duplicado
        -- ou usar IF NOT EXISTS se a versão do seu Postgres for > 9.5)
        CREATE INDEX IF NOT EXISTS idx_invite_code ON artifacts(discord_invite_code);
        CREATE INDEX IF NOT EXISTS idx_server_id ON artifacts(discord_server_id);
        CREATE INDEX IF NOT EXISTS idx_risk_score ON artifacts(risk_score);
        `

	_, err := r.db.Exec(context.Background(), query)
	if err != nil {
		log.Printf("Atenção na criação da tabela (pode ser ignorado se já existir): %v", err)
	} else {
		log.Println("Schema do banco verificado/criado com sucesso.")
	}
	return nil
}

func (r *ArtifactRepository) Save(ctx context.Context, a Artifact) (string, error) {
	query := `
        INSERT INTO artifacts 
        (source_url, author_id, discord_invite_code, discord_server_name, discord_server_id, discord_member_count, raw_ocr_text, risk_score, processed_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
        ON CONFLICT (source_url, discord_invite_code) DO UPDATE 
        SET processed_at = NOW(),
            discord_member_count = EXCLUDED.discord_member_count
        RETURNING id
    `
	var id string

	err := r.db.QueryRow(ctx, query,
		a.SourceURL,
		a.AuthorID,
		a.DiscordInviteCode,
		a.DiscordServerName,
		a.DiscordServerID,
		a.DiscordMemberCount,
		a.RawOcrText,
		a.RiskScore,
	).Scan(&id)

	return id, err
}

func (r *ArtifactRepository) Close(ctx context.Context) {
	r.db.Close(ctx)
}
