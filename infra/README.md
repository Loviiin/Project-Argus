# Infrastructure (Docker Compose)

Services

- NATS JetStream: message broker
- Postgres: primary datastore
- Redis: cache
- Meilisearch: search index

Environment

- Compose reads variables from your shell or a `.env` file at the repo root
- Do not commit secrets; use `.env` locally. See `.env.example`

Start

```bash
docker compose up -d nats argus-db argus-cache argus-meili
```

Status

```bash
docker compose ps
docker ps --format "table {{.Names}}\t{{.Status}}" | grep banco-argus-dev
```

Stop

```bash
docker compose down
```

Reset data volumes

```bash
docker compose down -v
```
