#!/bin/bash

#  Script de Teste - Vision Service + Discovery
# ================================================

set -e

echo "🚀 Teste de Captcha - Vision + Discovery"
echo "========================================"
echo ""

# Cores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Funções auxiliares
check_command() {
    if ! command -v "$1" &> /dev/null; then
        echo -e "${RED} $1 não encontrado${NC}"
        echo -e "   Instale com: $2"
        return 1
    fi
    echo -e "${GREEN} $1 instalado${NC}"
    return 0
}

check_port() {
    # Tenta usar nc, senão usa curl como fallback
    if command -v nc &> /dev/null; then
        if nc -z localhost "$1" 2>/dev/null; then
            echo -e "${GREEN} Porta $1 aberta${NC}"
            return 0
        fi
    else
        # Fallback: usa curl ou timeout + bash
        if timeout 1 bash -c "cat < /dev/null > /dev/tcp/localhost/$1" 2>/dev/null; then
            echo -e "${GREEN} Porta $1 aberta${NC}"
            return 0
        fi
    fi
    echo -e "${RED} Porta $1 fechada${NC}"
    return 1
}

cleanup() {
    echo ""
    echo -e "${YELLOW}🧹 Limpando...${NC}"
    
    if [ -f /tmp/vision.pid ]; then
        kill $(cat /tmp/vision.pid) 2>/dev/null || true
        rm /tmp/vision.pid
        echo "  Vision Service parado"
    fi
    
    if [ -f /tmp/discovery.pid ]; then
        kill $(cat /tmp/discovery.pid) 2>/dev/null || true
        rm /tmp/discovery.pid
        echo "  Discovery parado"
    fi
}

trap cleanup EXIT

# 1. Verificações
echo -e "${BLUE}  Verificando dependências...${NC}"
echo ""

check_command "python3" "apt install python3 / brew install python3" || exit 1
check_command "go" "https://golang.org/dl/" || exit 1
check_command "docker" "https://docs.docker.com/get-docker/" || exit 1

echo ""

# 2. NATS
echo -e "${BLUE}  Verificando NATS...${NC}"
echo ""

if ! docker ps | grep -q nats; then
    echo -e "${YELLOW}  NATS não está rodando${NC}"
    echo "  Iniciando via Docker Compose..."
    
    # Tenta docker compose (v2) primeiro, depois docker-compose (v1)
    if command -v docker &> /dev/null && docker compose version &> /dev/null; then
        docker compose up -d nats
    elif command -v docker-compose &> /dev/null; then
        docker-compose up -d nats
    else
        echo -e "${RED} Docker Compose não encontrado${NC}"
        exit 1
    fi
    
    echo "  Aguardando NATS inicializar..."
    sleep 3
fi

if check_port 4222; then
    echo -e "${GREEN} NATS rodando em nats://localhost:4222${NC}"
else
    echo -e "${RED} Falha ao iniciar NATS${NC}"
    exit 1
fi

echo ""

# 3. Python deps
echo -e "${BLUE}  Verificando dependências Python...${NC}"
echo ""

cd services/vision
if ! python3 -c "import cv2, numpy, nats" 2>/dev/null; then
    echo -e "${YELLOW}  Instalando dependências Python...${NC}"
    pip3 install -r requirements.txt
fi
cd ../..

echo -e "${GREEN} Dependências Python OK${NC}"
echo ""

# 4. Go deps
echo -e "${BLUE}  Verificando dependências Go...${NC}"
echo ""

cd services/discovery
if [ ! -f "go.sum" ]; then
    echo -e "${YELLOW}  Instalando dependências Go...${NC}"
    go mod tidy
fi
cd ../..

echo -e "${GREEN} Dependências Go OK${NC}"
echo ""

# 5. Inicia Vision Service
echo -e "${BLUE}  Iniciando Vision Service...${NC}"
echo ""

cd services/vision
NATS_URL=nats://localhost:4222 python3 -m src.captcha_solver > /tmp/vision.log 2>&1 &
VISION_PID=$!
echo $VISION_PID > /tmp/vision.pid
cd ../..

sleep 3

if ps -p $VISION_PID > /dev/null; then
    echo -e "${GREEN} Vision Service rodando (PID: $VISION_PID)${NC}"
    echo "   Logs: tail -f /tmp/vision.log"
else
    echo -e "${RED} Falha ao iniciar Vision Service${NC}"
    echo "   Veja os logs: cat /tmp/vision.log"
    exit 1
fi

echo ""

# 6. Teste de conexão
echo -e "${BLUE}  Testando conexão NATS...${NC}"
echo ""

sleep 2
if grep -q "Conectado ao NATS" /tmp/vision.log; then
    echo -e "${GREEN} Vision conectado ao NATS${NC}"
else
    echo -e "${YELLOW}  Verifique logs: tail -f /tmp/vision.log${NC}"
fi

echo ""

# 7. Instruções finais
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN} Setup completo!${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "${BLUE}📋 Próximos passos:${NC}"
echo ""
echo "    Inicie o Discovery em outro terminal:"
echo "     ${YELLOW}cd services/discovery && go run main.go${NC}"
echo ""
echo "    Ou teste manualmente:"
echo "     ${YELLOW}make test-discovery-captcha${NC}"
echo ""
echo -e "${BLUE}📊 Monitoramento:${NC}"
echo ""
echo "  • Logs Vision:    ${YELLOW}tail -f /tmp/vision.log${NC}"
echo "  • Logs NATS:      ${YELLOW}docker logs -f nats${NC}"
echo "  • DevTools (Rod): ${YELLOW}http://localhost:9222${NC}"
echo ""
echo -e "${BLUE}🛑 Para parar tudo:${NC}"
echo ""
echo "  ${YELLOW}make test-captcha-stop${NC}  ou  ${YELLOW}Ctrl+C${NC}"
echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

# Mantém rodando e mostra logs
echo ""
echo -e "${BLUE}📝 Mostrando logs do Vision Service...${NC}"
echo -e "${BLUE}   (Ctrl+C para sair)${NC}"
echo ""

tail -f /tmp/vision.log
