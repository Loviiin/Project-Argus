# Architecture

Flow overview:
- A producer publishes scrape jobs to `jobs.scrape` or discovery tasks to `jobs.discovery`
- Scraper service processes tasks
- Parser service processes output and saves artifacts to Postgres

NATS subjects:
- jobs.scrape: example publisher in Scraper for URLs

Database schema:
- Table `artifacts` stores source_url, author_id, discord metadata, raw_ocr_text, risk_score, processed_at

Verification:
- Use `docker exec ... psql` to inspect `artifacts`
