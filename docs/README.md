# Architecture

Flow overview:
- A producer publishes image paths to NATS subject `jobs.analyse`
- Vision service pulls jobs from `jobs.analyse`, extracts text with PaddleOCR, and publishes JSON to `data.text_extracted`
- Parser service subscribes to `data.text_extracted`, finds Discord invites, queries Discord API for metadata, and saves artifacts to Postgres

NATS subjects:
- jobs.analyse: input for Vision, message body is a file path on disk
- data.text_extracted: payload `{ source_path, text_content }`
- jobs.scrape: example publisher in Scraper for URLs (not wired by default)

Database schema:
- Table `artifacts` stores source_url, author_id, discord metadata, raw_ocr_text, risk_score, processed_at

Verification:
- Use `docker exec ... psql` to inspect `artifacts`
