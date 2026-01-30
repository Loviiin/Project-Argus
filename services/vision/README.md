# Vision Service (Python + PaddleOCR)

Purpose

- Pulls image processing jobs from `jobs.analyse`
- Extracts text using PaddleOCR
- Publishes `{ source_path, text_content }` to `data.text_extracted`

Install and run

```bash
# Via Makefile (Root)
make run-vision

# Manual
cd services/vision
pip3 install -r requirements.txt
python src/main.py
```

Testing

```bash
# Process jobs (Mock NATS)
make test-vision-job

# Manual (Send fake payload to Parser)
make send-payload
```

Environment

- `NATS_URL` defaults to `nats://localhost:4222`

Notes

- Input jobs on `jobs.analyse` are plain file paths. The service loads the image from disk, extracts text, and handles basic validation.

Configuration

- Vision does not require the main config file. If you need to customize subjects or broker URL, use environment variables (e.g., `NATS_URL`). Do not commit secrets.
