# Project Argus

Project Argus is a small OSINT pipeline built of multiple services:

- Vision (Python + PaddleOCR): extracts text from images
- Parser (Go): finds Discord invites in text and persists metadata in Postgres
- Scraper (Node.js + Playwright): publishes scrape jobs to NATS (example publisher)
- Infra (Docker): NATS JetStream, Postgres, Redis, Meilisearch

This repo is set up to run inside a devcontainer on Ubuntu. The commands below assume you are in a devcontainer terminal at the workspace root.

## Quick Start

We use a `Makefile` to simplify the workflow.

### 1. Configure the Application

Copy the example configuration:

```bash
cp config/config.example.yaml config/config.yaml
```

Edit `config/config.yaml` with your credentials. **Do not commit secrets.**

### 2. Setup & Infrastructure

Install dependencies and start the Docker infrastructure:

```bash
make setup  # Installs deps for Go, Node, and Python
make up     # Starts NATS, Postgres, Redis, Meilisearch
```

### 3. Run Services

Open separate terminals for each service:

```bash
make run-parser   # Terminal 1: Parser Service (Go)
make run-scraper  # Terminal 2: Scraper Service (Node)
make run-vision   # Terminal 3: Vision Service (Python)
```

### 4. Testing

Send a test payload (simulates Vision output -> Parser):

```bash
make send-payload
```

### 5. Verification

Check if data was inserted into Postgres:

```bash
docker exec -i banco-argus-dev psql -U argus-user -d argus-post-db -c "SELECT source_url, discord_invite_code, LEFT(raw_ocr_text, 50) as preview FROM artifacts ORDER BY processed_at DESC LIMIT 5;"
```

## Available Make Commands

- `make up`: Start infrastructure
- `make down`: Stop infrastructure
- `make logs`: detailed logs of infra
- `make setup`: Install all dependencies
- `make run-parser`: Run Parser service
- `make run-scraper`: Run Scraper service
- `make run-vision`: Run Vision service
- `make send-payload`: Send test payload
- `make help`: List all commands

## Troubleshooting

- If Parser fails to connect to Postgres with a Unix socket error, ensure the DB URL uses `127.0.0.1` and not `localhost`.
- Ensure containers are running: `docker compose ps`
- If running in open-core mode, set credentials via env vars (e.g. `DATABASE_URL`) instead of committing them in `config.yaml`.
