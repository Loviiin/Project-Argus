# Configuration

Open Core policy: configurations and credentials are not committed. Use the example file and copy it locally:

```bash
cp config/config.example.yaml config/config.yaml
```

Example contents (placeholders only):

```yaml
app:
  env: "local"

discord:
  token: "your_token_here" # optional; leave empty for anonymous

database:
  url: "postgres://your_user:postgres_password@127.0.0.1:5432/argus-post-db?sslmode=disable"

targets:
  hashtags:
    - "funnycats"
    - "dailyvlog"
    - "tutorial"

meilisearch:
  host: "http://"
  key: "masterKey"
  index: "artifacts"
```

Environment variables can override settings. Parser enables `viper.AutomaticEnv()` with `.` replaced by `_`, so `DATABASE_URL` overrides `database.url`.
