# Project Argus

Project Argus is a small OSINT pipeline built of multiple services:

- Vision (Python + PaddleOCR): extracts text from images
- Parser (Go): finds Discord invites in text and persists metadata in Postgres
- Scraper (Node.js + Playwright): publishes scrape jobs to NATS (example publisher)
- Infra (Docker): NATS JetStream, Postgres, Redis, Meilisearch

This repo is set up to run inside a devcontainer on Ubuntu. The commands below assume you are in a devcontainer terminal at the workspace root.

## Quick Start

1. Start infrastructure

```bash
docker compose up -d nats argus-db argus-cache argus-meili
```

2. Verify Postgres is healthy

```bash
docker ps --format "table {{.Names}}\t{{.Status}}" | grep banco-argus-dev
```

3. Configure the app

Use the example file and copy it to your local config:

```bash
cp config/config.example.yaml config/config.yaml
```

Edit `config/config.yaml` with your credentials. Do not commit secrets. Example placeholders:

```yaml
database:
	url: "postgres://your_user:postgres_password@127.0.0.1:5432/argus-post-db?sslmode=disable"

discord:
	token: "your_token_here" # leave empty for anonymous mode

targets:
	hashtags:
		- "funnycats"
		- "dailyvlog"
		- "tutorial"
```

4. Run the Parser service (Go)

```bash
cd services/parser
go run cmd/main.go
```

5. Run the Vision service (Python)

```bash
cd services/vision
pip3 install -r requirements.txt
python src/main.py
```

6. Send a test payload (skip OCR, publish directly to NATS)

Open a second terminal:

```bash
cd services/vision
python test_payload.py
```

7. Verify rows in Postgres

```bash
docker exec -i banco-argus-dev psql -U argus-user -d argus-post-db -c "SELECT source_url, discord_server_name, discord_invite_code, discord_member_count, LEFT(raw_ocr_text, 80) AS raw_preview, processed_at FROM artifacts ORDER BY processed_at DESC LIMIT 10;"
```

## Optional: Scraper publisher

Scraper publishes example URLs to NATS on `jobs.scrape`. It is a placeholder publisher and not wired to Vision by default.

```bash
cd services/scraper
npm install
npx ts-node src/start-pipeline.ts
```

## Environment

- NATS URL defaults to `nats://localhost:4222`; override via `NATS_URL`
- Config is loaded from `config/config.yaml`. Parser searches `../../config`, `/app/config`, and `.`

## Troubleshooting

- If Parser fails to connect to Postgres with a Unix socket error, ensure the DB URL uses `127.0.0.1` and not `localhost`.
- Ensure containers are running: `docker compose ps`
- If running in open-core mode, set credentials via env vars (e.g. `DATABASE_URL`) instead of committing them in `config.yaml`.
