package api

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// handleExport implementation
func (s *Server) handleExportImpl(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format != "csv" && format != "json" {
		http.Error(w, "format deve ser csv ou json", http.StatusBadRequest)
		return
	}

	// Filtros iguais aos de listagem/busca
	statusFilter := r.URL.Query().Get("status")
	minMembers := parseIntWithBounds(r, "min_members", 0, 0)
	maxMembers := parseIntWithBounds(r, "max_members", 0, 0)
	tagFilter := r.URL.Query().Get("tag")
	q := r.URL.Query().Get("q")

	conditions := []string{}
	args := []interface{}{}

	queryStr := "SELECT MAX(a.id), MAX(a.source_url), MAX(a.author_id), GROUP_CONCAT(DISTINCT a.discord_invite_code), MAX(a.discord_server_name), MAX(a.discord_member_count), MAX(a.discord_status), MAX(a.processed_at), MAX(a.tags) FROM artifacts a"

	if q != "" {
		cleanQuery := strings.ReplaceAll(q, "\"", "")
		cleanQuery = strings.ReplaceAll(cleanQuery, "'", "")
		words := strings.Fields(cleanQuery)
		for i, word := range words {
			words[i] = `"` + word + `"*`
		}
		ftsQuery := strings.Join(words, " AND ")

		queryStr = "SELECT MAX(a.id), MAX(a.source_url), MAX(a.author_id), GROUP_CONCAT(DISTINCT a.discord_invite_code), MAX(a.discord_server_name), MAX(a.discord_member_count), MAX(a.discord_status), MAX(a.processed_at), MAX(a.tags) FROM artifacts a JOIN artifacts_fts fts ON a.id = fts.rowid"
		conditions = append(conditions, "fts MATCH ?")
		args = append(args, ftsQuery)
	}

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

	queryStr += " " + whereClause + " GROUP BY CASE WHEN a.discord_server_id = '' OR a.discord_server_id IS NULL THEN a.discord_invite_code ELSE a.discord_server_id END ORDER BY MAX(a.processed_at) DESC LIMIT 10000"

	rows, err := s.db.QueryContext(r.Context(), queryStr, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=export.csv")
		cw := csv.NewWriter(w)
		cw.Write([]string{"ID", "Source URL", "Author ID", "Invite Codes", "Server Name", "Member Count", "Status", "Processed At", "Tags"})
		for rows.Next() {
			var id int
			var sourceUrl, authorId, inviteCode, processedAt string
			var serverName, status, tags sql.NullString
			var memberCount sql.NullInt64

			if err := rows.Scan(&id, &sourceUrl, &authorId, &inviteCode, &serverName, &memberCount, &status, &processedAt, &tags); err == nil {
				cw.Write([]string{
					fmt.Sprint(id), sourceUrl, authorId, inviteCode, serverName.String, fmt.Sprint(memberCount.Int64), status.String, processedAt, tags.String,
				})
			}
		}
		cw.Flush()
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=export.json")
		var results []map[string]interface{}
		for rows.Next() {
			var id int
			var sourceUrl, authorId, inviteCode, processedAt string
			var serverName, status, tags sql.NullString
			var memberCount sql.NullInt64

			if err := rows.Scan(&id, &sourceUrl, &authorId, &inviteCode, &serverName, &memberCount, &status, &processedAt, &tags); err == nil {
				results = append(results, map[string]interface{}{
					"id": id, "source_url": sourceUrl, "author_id": authorId, "invite_codes": inviteCode, "server_name": serverName.String, "member_count": memberCount.Int64, "status": status.String, "processed_at": processedAt, "tags": tags.String,
				})
			}
		}
		json.NewEncoder(w).Encode(results)
	}
}

func (s *Server) handleGetTopContributorsImpl(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT author_id, COUNT(*) as total,
		(SELECT avatar_url FROM comments c WHERE c.nickname = a.author_id LIMIT 1) as avatar_url,
		(SELECT unique_id FROM comments c WHERE c.nickname = a.author_id LIMIT 1) as unique_id
		FROM artifacts a
		GROUP BY author_id
		ORDER BY total DESC
		LIMIT 10
	`
	rows, err := s.db.QueryContext(r.Context(), query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var authorId string
		var total int
		var avatarUrl, uniqueId sql.NullString
		if err := rows.Scan(&authorId, &total, &avatarUrl, &uniqueId); err == nil {
			results = append(results, map[string]interface{}{
				"author_id":  authorId,
				"total":      total,
				"avatar_url": avatarUrl.String,
				"unique_id":  uniqueId.String,
			})
		}
	}
	json.NewEncoder(w).Encode(results)
}

func (s *Server) handleGetStatsImpl(w http.ResponseWriter, r *http.Request) {
	var total, total24h int
	s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM artifacts").Scan(&total)
	s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM artifacts WHERE processed_at >= datetime('now', '-1 day')").Scan(&total24h)

	var active, pending, invalid, rateLimited int
	s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM artifacts WHERE UPPER(discord_status) = 'ACTIVE'").Scan(&active)
	s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM artifacts WHERE UPPER(discord_status) = 'PENDING'").Scan(&pending)
	s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM artifacts WHERE UPPER(discord_status) IN ('EXPIRED', 'INVALID')").Scan(&invalid)
	s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM artifacts WHERE UPPER(discord_status) = 'RATE_LIMITED'").Scan(&rateLimited)

	// top tags (simple split in go since sqlite split is hard)
	rows, _ := s.db.QueryContext(r.Context(), "SELECT tags FROM artifacts WHERE tags IS NOT NULL AND tags != ''")
	tagCounts := make(map[string]int)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var tags string
			if err := rows.Scan(&tags); err == nil {
				for _, t := range strings.Split(tags, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						tagCounts[t]++
					}
				}
			}
		}
	}

	type tagCount struct {
		Tag   string `json:"tag"`
		Count int    `json:"count"`
	}
	var topTags []tagCount
	for k, v := range tagCounts {
		topTags = append(topTags, tagCount{Tag: k, Count: v})
	}
	// Sort topTags (not really needed but doing basic sort here is hard, we can let frontend sort or do a simple bubble sort)
	for i := 0; i < len(topTags); i++ {
		for j := i + 1; j < len(topTags); j++ {
			if topTags[j].Count > topTags[i].Count {
				topTags[i], topTags[j] = topTags[j], topTags[i]
			}
		}
	}
	if len(topTags) > 5 {
		topTags = topTags[:5]
	}

	dlqCount := 0
	if s.nc != nil {
		if js, err := s.nc.JetStream(); err == nil {
			for stream := range js.Streams() {
				if strings.Contains(strings.ToLower(stream.Config.Name), "dlq") {
					dlqCount += int(stream.State.Msgs)
				}
			}
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total":     total,
		"total_24h": total24h,
		"status": map[string]int{
			"active":       active,
			"pending":      pending,
			"invalid":      invalid,
			"rate_limited": rateLimited,
		},
		"top_tags":  topTags,
		"dlq_count": dlqCount,
	})
}

func (s *Server) handleUpdateTagsImpl(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// URL: /api/artifacts/{id}/tags
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 || parts[4] != "tags" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	id := parts[3]

	var req struct {
		Tags string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	_, err := s.db.ExecContext(r.Context(), "UPDATE artifacts SET tags = ? WHERE id = ?", req.Tags, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleGetPrometheusMetricsImpl(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1:8084/metrics")
	if err != nil {
		http.Error(w, "metrics indisponíveis: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)

	// Copy response body to writer
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	
	// Append lightweight internal metrics
	reqCount := atomic.LoadUint64(&s.apiRequestsTotal)
	latCount := atomic.LoadUint64(&s.apiLatencyTotalMs)
	
	avgLat := uint64(0)
	if reqCount > 0 {
		avgLat = latCount / reqCount
	}
	
	uptime := time.Since(serverStartTime).Seconds()
	
	fmt.Fprintf(w, "# HELP argus_api_requests_total Total de requests na API interna\n")
	fmt.Fprintf(w, "# TYPE argus_api_requests_total counter\n")
	fmt.Fprintf(w, "argus_api_requests_total %d\n\n", reqCount)

	fmt.Fprintf(w, "# HELP argus_api_latency_avg_ms Latência média de resposta da API (ms)\n")
	fmt.Fprintf(w, "# TYPE argus_api_latency_avg_ms gauge\n")
	fmt.Fprintf(w, "argus_api_latency_avg_ms %d\n\n", avgLat)
	
	fmt.Fprintf(w, "# HELP argus_parser_uptime_seconds Tempo de atividade do container (segundos)\n")
	fmt.Fprintf(w, "# TYPE argus_parser_uptime_seconds gauge\n")
	fmt.Fprintf(w, "argus_parser_uptime_seconds %.0f\n\n", uptime)
}
