.PHONY: all up down logs setup clean
.PHONY: setup-go setup-python
.PHONY: run-parser run-vision run-discovery run-captcha-solver
.PHONY: send-payload nats-start nats-stop nats-test
.PHONY: test-captcha-full test-captcha-stop test-discovery-captcha
.PHONY: build-discovery build-parser build-vision build-all

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


setup-python:
	@echo "Instalando deps do Vision (Python)..."
	cd services/vision && pip3 install torch torchvision --index-url https://download.pytorch.org/whl/cpu
	cd services/vision && pip3 install -r requirements.txt

# --- Run Services ---

run-parser: ## Roda o serviço Parser (Go)
	cd services/parser && go run cmd/main.go

run-discovery: ## Roda o serviço Discovery (Go)
	cd services/discovery && go run main.go

run-vision: ## Roda o serviço Vision (Python)
	cd services/vision && python3 src/main.py

run-captcha-solver: ## Roda o Captcha Solver (Vision com OpenCV)
	cd services/vision && python3 -m src.captcha_solver

# --- NATS ---

nats-start: ## Inicia o servidor NATS (se não estiver no Docker)
	@echo "Iniciando NATS Server na porta 4222..."
	@if command -v nats-server > /dev/null; then \
		nats-server -p 4222 & \
		echo "NATS rodando em nats://localhost:4222"; \
	else \
		echo "❌ nats-server não encontrado. Instale com:"; \
		echo "   - Mac: brew install nats-server"; \
		echo "   - Linux: curl -L https://github.com/nats-io/nats-server/releases/download/v2.10.7/nats-server-v2.10.7-linux-amd64.tar.gz | tar xz"; \
		echo "   - Ou use Docker: docker run -p 4222:4222 nats:latest"; \
	fi

nats-stop: ## Para o servidor NATS
	@pkill -f nats-server || echo "NATS não estava rodando"

nats-test: ## Testa conexão com NATS
	@echo "Testando conexão NATS..."
	@if command -v nats > /dev/null; then \
		nats pub test.topic "Hello NATS" && echo "✅ NATS OK"; \
	else \
		echo "❌ Cliente NATS não encontrado. Instale com:"; \
		echo "   - Mac: brew install nats-io/nats-tools/nats"; \
		echo "   - Linux: go install github.com/nats-io/natscli/nats@latest"; \
	fi

# --- Testing Captcha ---

test-captcha: ## 🚀 Teste AUTOMATIZADO completo (recomendado!)
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

test-captcha-stop: ## Para os serviços de teste
	@echo "🛑 Parando serviços de teste..."
	@if [ -f /tmp/vision.pid ]; then \
		kill $$(cat /tmp/vision.pid) 2>/dev/null || true; \
		rm /tmp/vision.pid; \
		echo "✅ Vision Service parado"; \
	fi

test-vision-logs: ## Mostra logs do Vision Service
	@tail -f /tmp/vision.log

test-discovery-captcha: ## Testa Discovery com captcha (requer Vision rodando)
	@echo "🧪 Testando Discovery com Vision Service"
	@echo "========================================"
	@echo ""
	@echo "⚠️  Certifique-se que o Vision está rodando: make run-captcha-solver"
	@echo ""
	@cd services/discovery && NATS_URL=nats://localhost:4222 go run main.go

# --- Build ---

build-discovery: ## Compila o serviço Discovery
	cd services/discovery && go build -o discovery main.go
	@echo "✅ Binário criado: services/discovery/discovery"

build-parser: ## Compila o serviço Parser
	cd services/parser && go build -o parser cmd/main.go
	@echo "✅ Binário criado: services/parser/parser"

build-vision: ## Build Docker image do Vision
	docker build -t argus-vision:latest services/vision
	@echo "✅ Imagem criada: argus-vision:latest"

build-all: build-discovery build-parser build-vision ## Compila todos os serviços

# --- Testing / Payload ---

test-full: ## Roda o teste de fluxo completo (Integration)
	python3 tests/integration/test_full_flow.py


test-vision-job: ## Roda o teste de job do vision (Mock NATS)
	python3 services/vision/tests/test_vision_job.py

send-payload: ## Envia um payload de teste (Vision -> Parser)
	python3 services/vision/tests/test_vision_payload.py

# --- Utilities ---

help: ## Mostra este help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

vnc: ## 🖥️  Reinicia o servidor noVNC (acesse http://localhost:6080 no browser)
	@bash /usr/local/bin/start-vnc.sh

