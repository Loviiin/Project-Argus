package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
)

type Server struct {
	db *sql.DB
}

func NewServer(db *sql.DB) *Server {
	return &Server{db: db}
}

func (s *Server) Start(port string) error {
	mux := http.NewServeMux()

	// Endpoints da API
	mux.HandleFunc("/api/artifacts", s.handleGetArtifacts)
	mux.HandleFunc("/api/comments", s.handleGetComments)

	// Arquivos estáticos (Dashboard)
	fs := http.FileServer(http.Dir("./internal/dashboard"))
	mux.Handle("/", fs)

	log.Printf("Iniciando Web Dashboard na porta %s", port)
	return http.ListenAndServe(port, mux)
}

func (s *Server) handleGetArtifacts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Pega limite e offset
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	statusFilter := r.URL.Query().Get("status")
	
	whereClause := ""
	if statusFilter == "VALID" {
		whereClause = "WHERE UPPER(a.discord_status) = 'ACTIVE'"
	} else if statusFilter == "PENDING" {
		whereClause = "WHERE UPPER(a.discord_status) = 'PENDING'"
	} else if statusFilter == "EXPIRED" {
		whereClause = "WHERE UPPER(a.discord_status) IN ('EXPIRED', 'INVALID')"
	} else if statusFilter == "RATE_LIMITED" {
		whereClause = "WHERE UPPER(a.discord_status) = 'RATE_LIMITED'"
	}

	query := fmt.Sprintf(`
		SELECT 
			MAX(a.id), MAX(a.source_url), MAX(a.author_id), GROUP_CONCAT(DISTINCT a.discord_invite_code), 
			MAX(a.discord_server_name), MAX(a.discord_member_count), MAX(a.discord_icon), 
			MAX(a.discord_status), MAX(a.processed_at), MAX(a.raw_ocr_text),
			MAX((SELECT avatar_url FROM comments c WHERE c.nickname = a.author_id LIMIT 1)) as avatar_url,
			COUNT(*) as mentions_count
		FROM artifacts a
		%s
		GROUP BY CASE WHEN a.discord_server_id = '' OR a.discord_server_id IS NULL THEN a.discord_invite_code ELSE a.discord_server_id END
		ORDER BY MAX(a.processed_at) DESC
		LIMIT $1 OFFSET $2
	`, whereClause)

	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []map[string]interface{}
	if results == nil {
		results = make([]map[string]interface{}, 0)
	}

	for rows.Next() {
		var id int
		var sourceUrl, authorId, inviteCode, processedAt string
		var rawOcr sql.NullString
		var serverName, icon, status sql.NullString
		var memberCount sql.NullInt64
		var avatarUrl sql.NullString

		var mentionsCount int

		if err := rows.Scan(&id, &sourceUrl, &authorId, &inviteCode, &serverName, &memberCount, &icon, &status, &processedAt, &rawOcr, &avatarUrl, &mentionsCount); err != nil {
			log.Println("Erro lendo row de artifacts:", err)
			continue
		}

		results = append(results, map[string]interface{}{
			"id":                   id,
			"source_url":           sourceUrl,
			"author_id":            authorId,
			"avatar_url":           avatarUrl.String,
			"discord_invite_codes": inviteCode,
			"discord_server_name":  serverName.String,
			"discord_member_count": memberCount.Int64,
			"discord_icon":         icon.String,
			"discord_status":       status.String,
			"processed_at":         processedAt,
			"raw_ocr_text":         rawOcr.String,
			"mentions_count":       mentionsCount,
		})
	}

	json.NewEncoder(w).Encode(results)
}

func (s *Server) handleGetComments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	query := `
		SELECT cid, aweme_id, text, digg_count, nickname, unique_id, avatar_url, created_at, reply_id
		FROM comments
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []map[string]interface{}
	if results == nil {
		results = make([]map[string]interface{}, 0)
	}

	for rows.Next() {
		var cid, awemeId, text, nickname, createdAt string
		var diggCount int
		var avatarUrl, replyId, uniqueId sql.NullString

		if err := rows.Scan(&cid, &awemeId, &text, &diggCount, &nickname, &uniqueId, &avatarUrl, &createdAt, &replyId); err != nil {
			log.Println("Erro lendo row de comments:", err)
			continue
		}

		results = append(results, map[string]interface{}{
			"cid":        cid,
			"aweme_id":   awemeId,
			"text":       text,
			"digg_count": diggCount,
			"nickname":   nickname,
			"unique_id":  uniqueId.String,
			"avatar_url": avatarUrl.String,
			"created_at": createdAt,
			"reply_id":   replyId.String,
		})
	}

	json.NewEncoder(w).Encode(results)
}
