.PHONY: all up down logs setup clean clean-data clean-workers
.PHONY: setup-go setup-python setup-discovery setup-scraper
.PHONY: run-parser run-vision run-discovery run-captcha-solver
.PHONY: run-worker-1 run-worker-2 run-worker-3 run-worker-4 run-worker-5 run-worker-6
.PHONY: test test-unit test-dedup test-captcha train-vision
.PHONY: build-discovery build-scraper build-parser build-vision build-all
.PHONY: help vnc

# 🚀 Project Argus - Makefile
# ============================
#
# 🎯 Início Rápido:
#   1. make up                    # Inicia infraestrutura (NATS, Redis, PostgreSQL, Meilisearch)
#   2. make run-discovery         # Terminal 1: Inicia Discovery
#   3. make run-worker-1          # Terminal 2: Inicia Worker/Scraper
#   4. make run-parser            # Terminal 3: Inicia Parser
#
# 🧪 Testes:
#   make test                     # Roda todos os testes
#   make test-dedup               # Testa sistema de deduplicação
#   make test-unit                # Testa lógica do TikTok Discovery
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

clean-data: ## Apaga todos os volumes (DB, Redis, NATS, Meili) e recria a infra limpa
	docker-compose down -v
	docker-compose up -d
	@echo "🧹 Ambiente limpo e reiniciado com sucesso!"

logs: ## Mostra os logs da infraestrutura
	docker-compose logs -f

clean-workers: ## Limpa travas (locks) e processos presos do Chromium dos workers
	@echo "🧹 Limpando processos e locks dos workers..."
	-@pkill -f chrome 2>/dev/null || true
	-@pkill -f chromium 2>/dev/null || true
	-@rm -f services/scraper/browser_state_worker_*/SingletonLock
	-@rm -f services/scraper/browser_state_worker_*/SingletonCookie
	-@rm -f services/scraper/browser_state_worker_*/SingletonSocket
	@echo "✅ Limpeza concluída!"

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
	cd services/vision && python3 -m venv .venv
	cd services/vision && ./.venv/bin/pip install torch torchvision --index-url https://download.pytorch.org/whl/cpu
	cd services/vision && ./.venv/bin/pip install -r requirements.txt

# --- Tests ---

test: test-unit test-dedup ## Roda todos os testes

test-unit: ## Roda testes unitários do Discovery (TikTok)
	@echo "🧪 Rodando testes unitários..."
	cd services/discovery/internal/sources/tiktok && go test -v

test-dedup: ## Testa sistema de deduplicação (Redis)
	@echo "🧪 Testando sistema de deduplicação..."
	./scripts/test-deduplication.sh

test-captcha: ## Teste rápido de captcha (apenas script)
	@./scripts/test-captcha.sh

# --- Run Services ---

run-parser: ## Roda o serviço Parser (Go)
	cd services/parser && go run cmd/main.go

run-discovery: ## Roda o serviço Discovery (Publisher)
	cd services/discovery && go run main.go

# Workers/Scrapers (pode rodar múltiplos em paralelo)

run-worker-1: ## Roda Worker/Scraper 1
	cd services/scraper && WORKER_ID=1 go run main.go

run-worker-2: ## Roda Worker/Scraper 2
	cd services/scraper && WORKER_ID=2 go run main.go

run-worker-3: ## Roda Worker/Scraper 3
	cd services/scraper && WORKER_ID=3 go run main.go

run-worker-4: ## Roda Worker/Scraper 4
	cd services/scraper && WORKER_ID=4 go run main.go

run-worker-5: ## Roda Worker/Scraper 5
	cd services/scraper && WORKER_ID=5 go run main.go

run-worker-6: ## Roda Worker/Scraper 6
	cd services/scraper && WORKER_ID=6 go run main.go

run-vision: ## Roda o serviço Vision (ML)
	cd services/vision && ./.venv/bin/python src/main.py

run-captcha-solver: ## Roda o Captcha Solver (Vision)
	cd services/vision && ./.venv/bin/python -m src.captcha_solver

# --- Vision/ML ---

train-vision: ## Treina o modelo ML do Captcha de Rotação
	cd services/vision && ./.venv/bin/python scripts/train.py

# --- Build ---

build-discovery: ## Compila o Discovery
	cd services/discovery && go build -o discovery main.go

build-scraper: ## Compila o Scraper
	cd services/scraper && go build -o scraper main.go

build-parser: ## Compila o Parser
	cd services/parser && go build -o parser cmd/main.go

build-vision: ## Builda a imagem Docker do Vision
	docker build -t argus-vision:latest services/vision

build-all: build-discovery build-scraper build-parser build-vision ## Compila todos os serviços

# --- Utilities ---

vnc: ## Reinicia o VNC (para debug visual)
	@echo "♻️  Reiniciando VNC..."
	-@pkill -f start-vnc.sh 2>/dev/null || true
	-@pkill -f Xvfb 2>/dev/null || true
	-@pkill -f x11vnc 2>/dev/null || true
	@bash /usr/local/bin/start-vnc.sh

help: ## Mostra esta mensagem de ajuda
	@echo ""
	@echo "🚀 Project Argus - Comandos Disponíveis"
	@echo "========================================"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'
	@echo ""
