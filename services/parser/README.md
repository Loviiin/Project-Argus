# Parser Service (Go)

Purpose

- Subscribes to `data.text_extracted`
- Parses text to find Discord invites and fetches metadata
- Persists artifacts in Postgres

Run

```bash
cd services/parser
go run cmd/main.go
```

Prerequisites

- Postgres running from `docker-compose.yml`
- NATS running
- Config file at `config/config.yaml` with a valid `database.url`. Do not commit real credentials; use environment variables (e.g., `DATABASE_URL`).

Environment

- `NATS_URL` defaults to `nats://localhost:4222`
- Discord token is optional; add `discord.token` in config for authenticated lookups

Verify persistence
Configuration

Use the provided example and copy it locally:

```bash
cp config/config.example.yaml config/config.yaml
```

Fill placeholders with your own values. Do not commit secrets. Example:

```yaml
database:
	url: "postgres://your_user:postgres_password@127.0.0.1:5432/argus-post-db?sslmode=disable"

discord:
	token: "your_token_here" # optional
```

```bash
docker exec -i banco-argus-dev psql -U argus-user -d argus-post-db -c "SELECT source_url, discord_server_name, discord_invite_code, discord_member_count, LEFT(raw_ocr_text, 80) AS raw_preview, processed_at FROM artifacts ORDER BY processed_at DESC LIMIT 10;"
```
