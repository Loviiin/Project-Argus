# Vision Service (Python + PaddleOCR)

Purpose

- Pulls image processing jobs from `jobs.analyse`
- Extracts text using PaddleOCR
- Publishes `{ source_path, text_content }` to `data.text_extracted`

Install and run

```bash
cd services/vision
pip3 install -r requirements.txt
python src/main.py
```

Quick test (publish a fake payload)

```bash
cd services/vision
python test_payload.py
```

Environment

- `NATS_URL` defaults to `nats://localhost:4222`

Notes

- Input jobs on `jobs.analyse` are plain file paths. The service loads the image from disk, extracts text, and handles basic validation.

Configuration

- Vision does not require the main config file. If you need to customize subjects or broker URL, use environment variables (e.g., `NATS_URL`). Do not commit secrets.
