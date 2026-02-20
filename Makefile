.PHONY: all up down logs setup clean
.PHONY: setup-go setup-python setup-discovery setup-scraper
.PHONY: run-parser run-vision run-discovery run-scraper run-captcha-solver
.PHONY: send-payload nats-start nats-stop nats-test
.PHONY: test-captcha test-captcha-full test-captcha-stop test-discovery-captcha
.PHONY: build-discovery build-scraper build-parser build-vision build-all

# 🚀 Project Argus - Makefile
# ============================
#
# 🧪 Teste Rápido de Captcha (Vision + Discovery):
#   1. make up                    # Inicia infraestrutura (NATS, Redis, etc)
#   2. make run-captcha-solver    # Terminal 1: Inicia Vision Service
#   3. make run-discovery         # Terminal 2: Inicia Discovery
#
# 📚 Comandos disponíveis:
#   make help                     # Lista todos os comandos
#
# ============================

# --- Infrastructure ---

up: ## Sobe a infraestrutura (Docker)
	docker-compose up -d
	@echo "✅ Infraestrutura iniciada"
	@echo "   NATS: nats://localhost:4222"
	@echo "   Redis: localhost:6379"
	@echo "   PostgreSQL: localhost:5432"
	@echo "   Meilisearch: http://localhost:7700"

down: ## Derruba a infraestrutura
	docker-compose down

logs: ## Mostra os logs da infraestrutura
	docker-compose logs -f

# --- Setup & Dependencies ---

setup: setup-go setup-discovery setup-python ## Instala dependências de todos os serviços
	@echo "Setup concluído!"

setup-go:
	@echo "Instalando deps do Parser (Go)..."
	cd services/parser && go mod tidy

setup-discovery:
	@echo "Instalando deps do Discovery (Go)..."
	cd services/discovery && go mod tidy

setup-scraper:
	@echo "Instalando deps do Scraper (Go)..."
	cd services/scraper && go mod tidy


setup-python:
	@echo "Instalando deps do Vision (Python)..."
	cd services/vision && pip3 install torch torchvision --index-url https://download.pytorch.org/whl/cpu
	cd services/vision && pip3 install -r requirements.txt

# --- Run Services ---

run-parser: ## Roda o serviço Parser (Go)
	cd services/parser && go run cmd/main.go

run-discovery: ## Roda o serviço Discovery (Publisher)
	cd services/discovery && go run main.go

run-scraper: ## Roda o Scraper Worker (Subscriber)
	cd services/scraper && go run main.go

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

test-captcha-full: ## Teste completo: NATS + Vision + Discovery (manual)
	@echo "🧪 Teste Completo de Captcha"
	@echo "================================"
	@echo ""
	@echo "1️⃣  Verificando NATS..."
	@docker ps | grep nats > /dev/null || (echo "❌ NATS não está rodando. Execute: make up" && exit 1)
	@echo "✅ NATS rodando"
	@echo ""
	@echo "2️⃣  Iniciando Vision Service (background)..."
	@cd services/vision && NATS_URL=nats://localhost:4222 python3 -m src.captcha_solver > /tmp/vision.log 2>&1 & echo $$! > /tmp/vision.pid
	@sleep 2
	@echo "✅ Vision Service iniciado (PID: $$(cat /tmp/vision.pid))"
	@echo ""
	@echo "3️⃣  Testando Discovery (simulação)..."
	@echo "   Para testar de verdade, execute manualmente:"
	@echo "   cd services/discovery && go run main.go"
	@echo ""
	@echo "📝 Logs do Vision: tail -f /tmp/vision.log"
	@echo "🛑 Para parar: make test-captcha-stop"

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

build-scraper:
	cd services/scraper && go build -o scraper main.go

build-parser:
	cd services/parser && go build -o parser cmd/main.go

build-vision:
	docker build -t argus-vision:latest services/vision

build-all: build-discovery build-scraper build-parser build-vision

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
