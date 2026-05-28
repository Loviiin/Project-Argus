package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
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
	DiscordIcon        string
	DiscordStatus      string
}

type Repository interface {
	Save(ctx context.Context, a Artifact) (string, error)
	UpdateEnrichedData(ctx context.Context, inviteCode, serverName, serverID, icon string, memberCount int, status string) error
	UpdateStatus(ctx context.Context, inviteCode, status string) error
	Close(ctx context.Context)
}

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(databaseURL string) (*PostgresRepository, error) {
	conn, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar no postgres: %w", err)
	}

	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("banco não responde: %w", err)
	}

	repo := &PostgresRepository{db: conn}

	if err := repo.runMigrations(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("falha ao rodar migrations: %w", err)
	}

	return repo, nil
}

func (r *PostgresRepository) Save(ctx context.Context, a Artifact) (string, error) {
	query := `
        INSERT INTO artifacts 
        (source_url, author_id, discord_invite_code, discord_server_name, discord_server_id, discord_member_count, raw_ocr_text, risk_score, processed_at,discord_icon, discord_status)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, $10)
        ON CONFLICT (source_url, discord_invite_code) DO UPDATE SET 
		 raw_ocr_text = artifacts.raw_ocr_text || ' | ' || EXCLUDED.raw_ocr_text,
         processed_at = NOW(),
            discord_member_count = EXCLUDED.discord_member_count,
			discord_status = EXCLUDED.discord_status
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
		a.DiscordIcon,
		a.DiscordStatus,
	).Scan(&id)

	return id, err
}

// UpdateEnrichedData atualiza SOMENTE os dados enriquecidos do Discord,
// sem tocar nos campos que já existem (source_url, raw_ocr_text, etc).
func (r *PostgresRepository) UpdateEnrichedData(ctx context.Context, inviteCode, serverName, serverID, icon string, memberCount int, status string) error {
	query := `
		UPDATE artifacts 
		SET discord_server_name = $2,
		    discord_server_id = $3,
		    discord_icon = $4,
		    discord_member_count = $5,
			discord_status = $6
		WHERE discord_invite_code = $1
	`
	_, err := r.db.Exec(ctx, query, inviteCode, serverName, serverID, icon, memberCount, status)
	return err
}

func (r *PostgresRepository) UpdateStatus(ctx context.Context, inviteCode, status string) error {
	query := `
		UPDATE artifacts 
		SET discord_status = $2
		WHERE discord_invite_code = $1
	`
	_, err := r.db.Exec(ctx, query, inviteCode, status)
	return err
}

func (r *PostgresRepository) Close(ctx context.Context) {
	r.db.Close()
}
