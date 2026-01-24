# Argus CLI Tool

Planned command-line interface for interacting with Project Argus.

Ideas:
- List recent artifacts from Postgres
- Publish test payloads to NATS subjects
- Simple health checks for infra components

Until the CLI is implemented, use Docker and psql to query artifacts:

```bash
docker exec -i banco-argus-dev psql -U argus-user -d argus-post-db -c "SELECT source_url, discord_server_name, discord_invite_code, processed_at FROM artifacts ORDER BY processed_at DESC LIMIT 10;"
```
