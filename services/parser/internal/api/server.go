package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/nats-io/nats.go"
)

type Server struct {
	db *sql.DB
	nc *nats.Conn
}

func NewServer(db *sql.DB, nc *nats.Conn) *Server {
	return &Server{db: db, nc: nc}
}

func (s *Server) SetNATS(nc *nats.Conn) {
	s.nc = nc
}

func (s *Server) Start(port string) error {
	mux := http.NewServeMux()

	// Endpoints da API
	mux.HandleFunc("/api/artifacts", s.handleGetArtifacts)
	mux.HandleFunc("/api/comments", s.handleGetComments)
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/api/export", s.handleExport)
	mux.HandleFunc("/api/stats/top-contributors", s.handleGetTopContributors)
	mux.HandleFunc("/api/stats", s.handleGetStats)
	// Como mux padrão do Go 1.22 aceita métodos, podemos fazer:
	// Mas como pode ser 1.21, vamos usar HandleFunc e tratar método dentro
	mux.HandleFunc("/api/artifacts/", s.handleUpdateTags)

	// Arquivos estáticos (Dashboard)
	fs := http.FileServer(http.Dir("./internal/dashboard"))
	mux.Handle("/", fs)

	limiter := NewIPRateLimiterFromEnv()

	slog.Info("Iniciando Web Dashboard", "port", port)
	return http.ListenAndServe(port, limiter.Middleware(mux))
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
	// val == 0 também cai no fallback: limit=0 causaria divisão por zero no cálculo de páginas
	if err != nil || val <= 0 {
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
	tagFilter := r.URL.Query().Get("tag")

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

	if tagFilter != "" {
		conditions = append(conditions, "a.tags LIKE ?")
		args = append(args, "%"+tagFilter+"%")
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

	sortOrder := r.URL.Query().Get("sort")
	sortBy := r.URL.Query().Get("sort_by")
	
	orderField := "MAX(a.processed_at)"
	if sortBy == "members" {
		orderField = "MAX(a.discord_member_count)"
	} else if sortBy == "name" {
		orderField = "MAX(a.discord_server_name)"
	} else if sortBy == "status" {
		orderField = "MAX(a.discord_status)"
	}
	
	orderClause := "ORDER BY " + orderField + " DESC"
	if sortOrder == "asc" {
		orderClause = "ORDER BY " + orderField + " ASC"
	}

	query := fmt.Sprintf(`
		SELECT 
			MAX(a.id), MAX(a.source_url), MAX(a.author_id), GROUP_CONCAT(DISTINCT a.discord_invite_code), 
			MAX(a.discord_server_name), MAX(a.discord_member_count), MAX(a.discord_icon), 
			MAX(a.discord_status), MAX(a.processed_at), MAX(a.raw_ocr_text),
			(SELECT avatar_url FROM comments c WHERE c.nickname = MAX(a.author_id) LIMIT 1) as avatar_url,
			COUNT(*) as mentions_count, MAX(a.tags) as tags
		FROM artifacts a
		%s
		GROUP BY CASE WHEN a.discord_server_id = '' OR a.discord_server_id IS NULL THEN a.discord_invite_code ELSE a.discord_server_id END
		%s
		LIMIT ? OFFSET ?
	`, whereClause, orderClause)

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
		var serverName, icon, status, tags sql.NullString
		var memberCount sql.NullInt64
		var avatarUrl sql.NullString

		var mentionsCount int

		if err := rows.Scan(&id, &sourceUrl, &authorId, &inviteCode, &serverName, &memberCount, &icon, &status, &processedAt, &rawOcr, &avatarUrl, &mentionsCount, &tags); err != nil {
			slog.Error("Erro lendo row de artifacts", "error", err)
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
			"tags":                 tags.String,
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
			slog.Error("Erro lendo row de comments", "error", err)
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

	// Remover aspas para evitar erro de sintaxe no parser do FTS5 (malformed MATCH expression)
	cleanQuery := strings.ReplaceAll(query, "\"", "")
	cleanQuery = strings.ReplaceAll(cleanQuery, "'", "")

	// Converter "Fla" para '"Fla"*' para permitir busca por prefixo no FTS5
	words := strings.Fields(cleanQuery)
	for i, word := range words {
		words[i] = `"` + word + `"*`
	}
	ftsQuery := strings.Join(words, " AND ")

	limit := parseIntWithBounds(r, "limit", 50, 200)
	offset := parseIntWithBounds(r, "offset", 0, 0)
	statusFilter := r.URL.Query().Get("status")
	minMembers := parseIntWithBounds(r, "min_members", 0, 0)
	maxMembers := parseIntWithBounds(r, "max_members", 0, 0)
	tagFilter := r.URL.Query().Get("tag")

	conditions := []string{"artifacts_fts MATCH ?"}
	args := []interface{}{ftsQuery}

	if statusFilter == "VALID" {
		conditions = append(conditions, "UPPER(a.discord_status) = 'ACTIVE'")
	} else if statusFilter == "PENDING" {
		conditions = append(conditions, "UPPER(a.discord_status) = 'PENDING'")
	} else if statusFilter == "EXPIRED" {
		conditions = append(conditions, "UPPER(a.discord_status) IN ('EXPIRED', 'INVALID')")
	} else if statusFilter == "RATE_LIMITED" {
		conditions = append(conditions, "UPPER(a.discord_status) = 'RATE_LIMITED'")
	}

	if tagFilter != "" {
		conditions = append(conditions, "a.tags LIKE ?")
		args = append(args, "%"+tagFilter+"%")
	}

	if minMembers > 0 {
		conditions = append(conditions, "a.discord_member_count >= ?")
		args = append(args, minMembers)
	}
	if maxMembers > 0 {
		conditions = append(conditions, "a.discord_member_count <= ?")
		args = append(args, maxMembers)
	}

	whereClause := "WHERE " + strings.Join(conditions, " AND ")

	var totalCount int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM artifacts a JOIN artifacts_fts fts ON a.id = fts.rowid %s", whereClause)
	s.db.QueryRowContext(r.Context(), countQuery, args...).Scan(&totalCount)

	sortOrder := r.URL.Query().Get("sort")
	sortBy := r.URL.Query().Get("sort_by")
	
	orderField := "a.processed_at"
	if sortBy == "members" {
		orderField = "a.discord_member_count"
	} else if sortBy == "name" {
		orderField = "a.discord_server_name"
	} else if sortBy == "status" {
		orderField = "a.discord_status"
	}
	
	orderClause := "ORDER BY " + orderField + " DESC" // default to recency for search too
	if sortOrder == "asc" {
		orderClause = "ORDER BY " + orderField + " ASC"
	}

	queryStr := fmt.Sprintf(`
		SELECT a.id, a.source_url, a.discord_invite_code, a.author_id,
			   a.discord_server_name, a.discord_server_id,
			   a.discord_member_count, a.discord_icon,
			   a.discord_status, a.processed_at,
			   (SELECT avatar_url FROM comments c WHERE c.nickname = a.author_id LIMIT 1) as avatar_url,
			   a.tags
		FROM artifacts a
		JOIN artifacts_fts fts ON a.id = fts.rowid
		%s
		%s
		LIMIT ? OFFSET ?
	`, whereClause, orderClause)

	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(r.Context(), queryStr, args...)
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
		var authorId, serverName, serverId, icon, status, tags sql.NullString
		var memberCount sql.NullInt64
		var avatarUrl sql.NullString

		if err := rows.Scan(&id, &sourceUrl, &inviteCode, &authorId, &serverName, &serverId, &memberCount, &icon, &status, &processedAt, &avatarUrl, &tags); err != nil {
			slog.Error("Erro lendo row de search", "error", err)
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
			"tags":                 tags.String,
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

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	s.handleExportImpl(w, r)
}

func (s *Server) handleGetTopContributors(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	s.handleGetTopContributorsImpl(w, r)
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	s.handleGetStatsImpl(w, r)
}

func (s *Server) handleUpdateTags(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	s.handleUpdateTagsImpl(w, r)
}
