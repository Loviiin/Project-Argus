# Scraper Service (Node.js + Playwright)

Purpose

- Example publisher that sends URLs to NATS subject `jobs.scrape`
- Not wired to Vision by default; use it as a template for building ingestion

Install and run

```bash
# Via Makefile (Root)
make run-scraper

# Manual
cd services/scraper
npm install
npm start
```

Testing

```bash
# Via Makefile (Root)
make test-scraper-browser

# Manual
cd services/scraper
npx ts-node tests/browser.test.ts
```

Publish a single URL via pipeline starter

```bash
cd services/scraper
npx ts-node src/start-pipeline.ts
```

Environment

- `NATS_URL` defaults to `nats://localhost:4222` (configured in code)

Configuration

- Scraper is an example publisher. Configure via environment variables if needed. Do not commit secrets.
