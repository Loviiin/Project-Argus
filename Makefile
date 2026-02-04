.PHONY: all help
.PHONY: up down logs
.PHONY: setup setup-go setup-discovery setup-python
.PHONY: run-parser run-vision run-discovery run-captcha-solver
.PHONY: test-captcha test-captcha-stop
.PHONY: build-discovery build-parser build-vision build-all

.DEFAULT_GOAL := help

help: ##  Mostra todos os comandos disponíveis
	@echo " Project Argus - Makefile"
	@echo "================================"
	@echo ""
	@echo " Quick Start - Teste Captcha:"
	@echo "  1. make up                  → Inicia infraestrutura"
	@echo "  2. make test-captcha        → Teste automatizado completo"
	@echo ""
	@echo " Comandos:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'
	@echo ""

# ============================================
#   Infraestrutura
# ============================================

up: ##   Inicia infraestrutura completa
	@echo " Iniciando infraestrutura..."
	@docker-compose up -d
	@sleep 2
	@echo ""
	@echo " Serviços iniciados:"
	@docker-compose ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || docker-compose ps
	@echo ""
	@echo " URLs:"
	@echo "   NATS:        nats://localhost:4222"
	@echo "   Redis:       localhost:6379"
	@echo "   PostgreSQL:  localhost:5432"
	@echo "   Meilisearch: http://localhost:7700"

down: ##   Para infraestrutura
	@docker-compose down

logs: ##  Logs da infraestrutura
	@docker-compose logs -f

# ============================================
#  Setup & Dependências
# ============================================

setup: setup-go setup-discovery setup-python ##  Instala todas as dependências
	@echo ""
	@echo " Setup concluído!"

setup-go: ##  Setup Parser (Go)
	@echo " Instalando deps do Parser..."
	@cd services/parser && go mod tidy

setup-discovery: ##  Setup Discovery (Go)
	@echo " Instalando deps do Discovery..."
	@cd services/discovery && go mod tidy

setup-python: ##  Setup Vision (Python)
	@echo " Instalando deps do Vision..."
	@cd services/vision && pip3 install -q -r requirements.txt

# ============================================
#  Executar Serviços
# ============================================

run-parser: ##  Parser Service
	@cd services/parser && go run cmd/main.go

run-discovery: ##  Discovery Service (TikTok)
	@cd services/discovery && go run main.go

run-vision: ##  Vision Service
	@cd services/vision && python3 src/main.py

run-captcha-solver: ##  Captcha Solver (OpenCV)
	@cd services/vision && python3 -m src.captcha_solver

# ============================================
#  Testes
# ============================================

test-captcha: ##  Teste automatizado completo
	@./scripts/test-captcha.sh

test-captcha-stop: ##  Para testes em background
	@if [ -f /tmp/vision.pid ]; then \
		kill $$(cat /tmp/vision.pid) 2>/dev/null || true; \
		rm /tmp/vision.pid; \
		echo " Vision Service parado"; \
	else \
		echo "  Nenhum serviço rodando"; \
	fi

# ============================================
#  Build
# ============================================

build-discovery: ##  Build Discovery
	@echo " Compilando Discovery..."
	@cd services/discovery && go build -o discovery main.go
	@echo " services/discovery/discovery"

build-parser: ##  Build Parser
	@echo " Compilando Parser..."
	@cd services/parser && go build -o parser cmd/main.go
	@echo " services/parser/parser"

build-vision: ##  Build Vision (Docker)
	@echo " Buildando imagem Vision..."
	@docker build -t argus-vision:latest services/vision
	@echo " argus-vision:latest"

build-all: build-discovery build-parser build-vision ##  Build completo

# ============================================
#  Validação & Debug
# ============================================

validate: ##  Valida setup completo
	@echo " Validando ambiente..."
	@echo ""
	@echo " Docker:"
	@docker-compose ps --format "table {{.Name}}\t{{.Status}}" 2>/dev/null || echo "  ⚠️  Não iniciado (make up)"
	@echo ""
	@echo " Go:"
	@command -v go >/dev/null && echo "   $$(go version)" || echo "  ❌ Go não instalado"
	@echo ""
	@echo "🐍 Python:"
	@command -v python3 >/dev/null && echo "   $$(python3 --version)" || echo "  ❌ Python não instalado"
	@echo ""
	@echo " NATS:"
	@(timeout 1 bash -c 'cat < /dev/null > /dev/tcp/localhost/4222' 2>/dev/null && echo "   localhost:4222") || echo "  ⚠️  Porta 4222 não acessível"
	@echo ""
	@echo "💾 Redis:"
	@(timeout 1 bash -c 'cat < /dev/null > /dev/tcp/localhost/6379' 2>/dev/null && echo "   localhost:6379") || echo "  ⚠️  Porta 6379 não acessível"
