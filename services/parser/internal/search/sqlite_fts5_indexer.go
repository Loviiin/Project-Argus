package search

import (
	"database/sql"
	"fmt"
)

type SQLiteFTS5Indexer struct {
	db *sql.DB
}

func NewSQLiteFTS5Indexer(db *sql.DB) *SQLiteFTS5Indexer {
	return &SQLiteFTS5Indexer{db: db}
}

func (s *SQLiteFTS5Indexer) IndexData(doc map[string]interface{}) error {
	// No SQLite, a inserção principal já acontece via repository.Save,
	// e os triggers do FTS5 (artifacts_ai) sincronizam o texto automaticamente.
	// O IndexData aqui pode ser um no-op, ou atualizar campos adicionais se necessário.
	return nil
}

func (s *SQLiteFTS5Indexer) UpdateData(doc map[string]interface{}) error {
	// A atualização também já é feita via repository.UpdateEnrichedData.
	// Se houvesse campos adicionais exclusivos do Search, atualizaríamos aqui.
	return nil
}

func (s *SQLiteFTS5Indexer) GetDocument(pk string) (*SearchDoc, error) {
	query := `
		SELECT discord_invite_code, discord_server_name, source_url, processed_at, discord_member_count, discord_icon, discord_status
		FROM artifacts
		WHERE discord_invite_code = ?
	`
	row := s.db.QueryRow(query, pk)

	var doc SearchDoc
	var serverName, icon, status sql.NullString
	var processedAt sql.NullTime

	err := row.Scan(&doc.InviteCode, &serverName, &doc.SourceURL, &processedAt, &doc.MemberCount, &icon, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("documento não encontrado")
		}
		return nil, err
	}

	if serverName.Valid {
		doc.ServerName = serverName.String
	}
	if icon.Valid {
		doc.Icon = icon.String
	}
	if status.Valid {
		doc.Status = status.String
	}
	if processedAt.Valid {
		doc.TimestampFormatted = processedAt.Time.Format("02/01/2006 15:04:05")
	}

	return &doc, nil
}
