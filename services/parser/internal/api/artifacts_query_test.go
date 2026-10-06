package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
)

// Garante que a query agregada de /api/artifacts é válida no SQLite
// (regressão: "misuse of aggregate function MAX()").
func TestHandleGetArtifacts_SQLite(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE artifacts(id INTEGER PRIMARY KEY, source_url TEXT, author_id TEXT, discord_invite_code TEXT,
			discord_server_name TEXT, discord_member_count INT, discord_icon TEXT, discord_status TEXT,
			processed_at TEXT, raw_ocr_text TEXT, tags TEXT, discord_server_id TEXT);
		CREATE TABLE comments(nickname TEXT, avatar_url TEXT);
		INSERT INTO artifacts VALUES
			(1,'u','bob','abc','S',10,'i','ACTIVE','2026','t','g','s1'),
			(2,'u','bob','def','S',12,'i','ACTIVE','2027','t','g','s1'),
			(3,'u','al','xyz',NULL,NULL,NULL,'PENDING','2025',NULL,NULL,'');
		INSERT INTO comments VALUES('bob','http://av');
	`)
	if err != nil {
		t.Fatal(err)
	}

	s := NewServer(db, nil)
	for _, url := range []string{
		"/api/artifacts",
		"/api/artifacts?sort_by=members&sort=asc",
		"/api/artifacts?status=VALID&min_members=5",
	} {
		rec := httptest.NewRecorder()
		s.handleGetArtifacts(rec, httptest.NewRequest(http.MethodGet, url, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d body %s", url, rec.Code, rec.Body.String())
		}
		var resp PaginatedResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("%s: invalid JSON: %v", url, err)
		}
		if len(resp.Items) == 0 {
			t.Fatalf("%s: expected items", url)
		}
	}
}
