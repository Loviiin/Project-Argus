package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	mux.HandleFunc("/api/search", s.handleSearch)

	// Arquivos estáticos (Dashboard)
	fs := http.FileServer(http.Dir("./internal/dashboard"))
	mux.Handle("/", fs)

	log.Printf("Iniciando Web Dashboard na porta %s", port)
	return http.ListenAndServe(port, mux)
}

var allowedOrigin = getEnv("ALLOWED_ORIGIN", "*")

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type PaginatedResponse struct {
	Items      []map[string]interface{} `json:"items"`
	TotalCount int                      `json:"total_count"`
	Page       int                      `json:"page"`
	Limit      int                      `json:"limit"`
	TotalPages int                      `json:"total_pages"`
}

func setCORS(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func parseIntWithBounds(r *http.Request, key string, fallback, maxVal int) int {
	val, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil || val < 0 {
		val = fallback
	}
	if maxVal > 0 && val > maxVal {
		val = maxVal
	}
	return val
}

func (s *Server) handleGetArtifacts(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	limit := parseIntWithBounds(r, "limit", 50, 200)
	offset := parseIntWithBounds(r, "offset", 0, 0)
	statusFilter := r.URL.Query().Get("status")
	minMembers := parseIntWithBounds(r, "min_members", 0, 0)
	maxMembers := parseIntWithBounds(r, "max_members", 0, 0)

	conditions := []string{}
	args := []interface{}{}

	if statusFilter == "VALID" {
		conditions = append(conditions, "UPPER(a.discord_status) = 'ACTIVE'")
	} else if statusFilter == "PENDING" {
		conditions = append(conditions, "UPPER(a.discord_status) = 'PENDING'")
	} else if statusFilter == "EXPIRED" {
		conditions = append(conditions, "UPPER(a.discord_status) IN ('EXPIRED', 'INVALID')")
	} else if statusFilter == "RATE_LIMITED" {
		conditions = append(conditions, "UPPER(a.discord_status) = 'RATE_LIMITED'")
	}

	if minMembers > 0 {
		conditions = append(conditions, "a.discord_member_count >= ?")
		args = append(args, minMembers)
	}
	if maxMembers > 0 {
		conditions = append(conditions, "a.discord_member_count <= ?")
		args = append(args, maxMembers)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var totalCount int
	countQuery := fmt.Sprintf("SELECT COUNT(DISTINCT CASE WHEN a.discord_server_id = '' OR a.discord_server_id IS NULL THEN a.discord_invite_code ELSE a.discord_server_id END) FROM artifacts a %s", whereClause)
	s.db.QueryRowContext(r.Context(), countQuery, args...).Scan(&totalCount)

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
		LIMIT ? OFFSET ?
	`, whereClause)

	args = append(args, limit, offset)
	rows, err := s.db.Query(query, args...)
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
	setCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	limit := parseIntWithBounds(r, "limit", 50, 200)
	offset := parseIntWithBounds(r, "offset", 0, 0)

	var totalCount int
	s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM comments").Scan(&totalCount)

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

	page := (offset / limit) + 1
	totalPages := (totalCount + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	json.NewEncoder(w).Encode(PaginatedResponse{
		Items:      results,
		TotalCount: totalCount,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "missing query", http.StatusBadRequest)
		return
	}
	limit := parseIntWithBounds(r, "limit", 50, 200)
	offset := parseIntWithBounds(r, "offset", 0, 0)

	var totalCount int
	s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM artifacts_fts WHERE artifacts_fts MATCH ?", query).Scan(&totalCount)

	rows, err := s.db.QueryContext(r.Context(), `
		SELECT a.id, a.source_url, a.discord_invite_code, a.author_id,
			   a.discord_server_name, a.discord_server_id,
			   a.discord_member_count, a.discord_icon,
			   a.discord_status, a.processed_at,
			   (SELECT avatar_url FROM comments c WHERE c.nickname = a.author_id LIMIT 1) as avatar_url
		FROM artifacts a
		JOIN artifacts_fts fts ON a.id = fts.rowid
		WHERE artifacts_fts MATCH ?
		ORDER BY rank
		LIMIT ? OFFSET ?
	`, query, limit, offset)
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
		var sourceUrl, inviteCode, processedAt string
		var authorId, serverName, serverId, icon, status sql.NullString
		var memberCount sql.NullInt64
		var avatarUrl sql.NullString

		if err := rows.Scan(&id, &sourceUrl, &inviteCode, &authorId, &serverName, &serverId, &memberCount, &icon, &status, &processedAt, &avatarUrl); err != nil {
			log.Println("Erro lendo row de search:", err)
			continue
		}

		results = append(results, map[string]interface{}{
			"id":                   id,
			"source_url":           sourceUrl,
			"author_id":            authorId.String,
			"avatar_url":           avatarUrl.String,
			"discord_invite_codes": inviteCode,
			"discord_server_name":  serverName.String,
			"discord_server_id":    serverId.String,
			"discord_member_count": memberCount.Int64,
			"discord_icon":         icon.String,
			"discord_status":       status.String,
			"processed_at":         processedAt,
		})
	}

	page := (offset / limit) + 1
	totalPages := (totalCount + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	json.NewEncoder(w).Encode(PaginatedResponse{
		Items:      results,
		TotalCount: totalCount,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}
