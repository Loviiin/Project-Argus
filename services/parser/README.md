# Parser Service (Go)

Purpose

- Subscribes to `data.text_extracted`
- Parses text to find Discord invites and fetches metadata
- Persists artifacts in Postgres
- Indexes artifacts in Meilisearch for search

Run

```bash
cd services/parser
go run cmd/main.go
```

Prerequisites

- Postgres running from `docker-compose.yml`
- NATS running
- Meilisearch running
- Config file at `config/config.yaml` with valid `database.url` and `meilisearch` settings.

Environment

- `NATS_URL` defaults to `nats://localhost:4222`
- Discord token is optional; add `discord.token` in config for authenticated lookups

Configuration

Use the provided example and copy it locally:

```bash
cp config/config.example.yaml config/config.yaml
```

Fill placeholders with your own values. Do not commit secrets. Example:

```yaml
database:
	url: "postgres://your_user:postgres_password@127.0.0.1:5432/argus-post-db?sslmode=disable"

meilisearch:
    host: "http://localhost:7700"
    key: "masterKey123"
    index: "artifacts"

discord:
	token: "your_token_here" # optional
```

Verify persistence (Postgres)

```bash
docker exec -i banco-argus-dev psql -U argus-user -d argus-post-db -c "SELECT source_url, discord_server_name, discord_invite_code, discord_member_count, LEFT(raw_ocr_text, 80) AS raw_preview, processed_at FROM artifacts ORDER BY processed_at DESC LIMIT 10;"
```

Verify indexing (Meilisearch)

```bash
curl -X POST 'http://localhost:7700/indexes/artifacts/search' \
  -H 'Authorization: Bearer masterKey123' \
  -H 'Content-Type: application/json' \
  --data-binary '{ "q": "" }'
```
