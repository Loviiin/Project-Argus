.PHONY: all up down logs setup clean
.PHONY: setup-go setup-python
.PHONY: run-parser run-vision
.PHONY: send-payload

# --- Infrastructure ---

up: ## Sobe a infraestrutura (Docker)
	docker-compose up -d

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
