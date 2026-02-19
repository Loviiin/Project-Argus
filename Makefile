.PHONY: all up down logs setup clean
.PHONY: setup-go setup-python setup-discovery
.PHONY: run-parser run-vision run-discovery run-captcha-solver
.PHONY: send-payload nats-start nats-stop nats-test
.PHONY: test-captcha test-captcha-full test-captcha-stop test-discovery-captcha
.PHONY: build-discovery build-parser build-vision build-all

up:
	docker-compose up -d

down:
	docker-compose down

logs:
	docker-compose logs -f

setup: setup-go setup-discovery setup-python

setup-go:
	cd services/parser && go mod tidy

setup-discovery:
	cd services/discovery && go mod tidy

setup-python:
	cd services/vision && pip3 install torch torchvision --index-url https://download.pytorch.org/whl/cpu
	cd services/vision && pip3 install -r requirements.txt

run-parser:
	cd services/parser && go run cmd/main.go

run-discovery:
	cd services/discovery && go run main.go

run-vision:
	cd services/vision && python3 src/main.py

run-captcha-solver:
	cd services/vision && python3 -m src.captcha_solver

nats-start:
	@if command -v nats-server > /dev/null; then \
		nats-server -p 4222 & \
		echo "NATS running at nats://localhost:4222"; \
	else \
		echo "nats-server not found. Install with:"; \
		echo "  Linux: curl -L https://github.com/nats-io/nats-server/releases/latest/download/nats-server-linux-amd64.tar.gz | tar xz"; \
		echo "  Docker: docker run -p 4222:4222 nats:latest"; \
	fi

nats-stop:
	@pkill -f nats-server || echo "NATS was not running"

nats-test:
	@if command -v nats > /dev/null; then \
		nats pub test.topic "ping" && echo "NATS OK"; \
	else \
		echo "nats CLI not found. Install: go install github.com/nats-io/natscli/nats@latest"; \
	fi

test-captcha:
	@./scripts/test-captcha.sh

test-captcha-full:
	@echo "Checking NATS..."
	@docker ps | grep nats > /dev/null || (echo "NATS not running. Run: make up" && exit 1)
	@echo "NATS OK"
	@cd services/vision && NATS_URL=nats://localhost:4222 python3 -m src.captcha_solver > /tmp/vision.log 2>&1 & echo $$! > /tmp/vision.pid
	@sleep 2
	@echo "Vision started (PID: $$(cat /tmp/vision.pid))"
	@echo "Run discovery manually: cd services/discovery && go run main.go"
	@echo "Vision logs: tail -f /tmp/vision.log"
	@echo "Stop: make test-captcha-stop"

test-captcha-stop:
	@if [ -f /tmp/vision.pid ]; then \
		kill $$(cat /tmp/vision.pid) 2>/dev/null || true; \
		rm /tmp/vision.pid; \
		echo "Vision stopped"; \
	fi

test-vision-logs:
	@tail -f /tmp/vision.log

test-discovery-captcha:
	@cd services/discovery && NATS_URL=nats://localhost:4222 go run main.go

build-discovery:
	cd services/discovery && go build -o discovery main.go

build-parser:
	cd services/parser && go build -o parser cmd/main.go

build-vision:
	docker build -t argus-vision:latest services/vision

build-all: build-discovery build-parser build-vision

test-full:
	python3 tests/integration/test_full_flow.py

test-vision-job:
	python3 services/vision/tests/test_vision_job.py

send-payload:
	python3 services/vision/tests/test_vision_payload.py

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "%-22s %s\n", $$1, $$2}'

vnc:
	@bash /usr/local/bin/start-vnc.sh
