.PHONY: all up down logs setup clean
.PHONY: setup-go setup-node setup-python
.PHONY: run-parser run-scraper run-vision
.PHONY: send-payload

# --- Infrastructure ---

up: ## Sobe a infraestrutura (Docker)
	docker-compose up -d

down: ## Derruba a infraestrutura
	docker-compose down

logs: ## Mostra os logs da infraestrutura
	docker-compose logs -f

# --- Setup & Dependencies ---

setup: setup-go setup-node setup-python ## Instala dependências de todos os serviços
	@echo "Setup concluído!"

setup-go:
	@echo "Instalando deps do Parser (Go)..."
	cd services/parser && go mod tidy

setup-node:
	@echo "Instalando deps do Scraper (Node)..."
	cd services/scraper && npm install

setup-python:
	@echo "Instalando deps do Vision (Python)..."
	cd services/vision && pip3 install -r requirements.txt

# --- Run Services ---

run-parser: ## Roda o serviço Parser (Go)
	cd services/parser && go run cmd/main.go

run-scraper: ## Roda o serviço Scraper (Node)
	cd services/scraper && npm start

run-vision: ## Roda o serviço Vision (Python)
	cd services/vision && python3 src/main.py

# --- Testing / Payload ---

send-payload: ## Envia um payload de teste (Vision -> Parser)
	cd services/vision && python3 test_payload.py

# --- Utilities ---

help: ## Mostra este help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
