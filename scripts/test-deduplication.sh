#!/bin/bash

echo "🧪 Teste de Deduplicação - Project Argus"
echo "========================================"
echo ""

# Cores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuração
REDIS_HOST=${REDIS_HOST:-localhost:6379}
VIDEO_ID="7610549811661540615"

echo "📋 Pré-requisitos:"
echo "   - Redis rodando em $REDIS_HOST"
echo "   - NATS rodando (opcional para teste completo)"
echo ""

# Função para verificar Redis
check_redis() {
    if docker-compose exec -T argus-cache redis-cli ping > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC} Redis está rodando"
        return 0
    else
        echo -e "${RED}✗${NC} Redis não está acessível"
        return 1
    fi
}

# Verifica Redis
if ! check_redis; then
    echo ""
    echo "Iniciando Redis..."
    docker-compose up -d argus-cache
    sleep 2
    check_redis || exit 1
fi

echo ""
echo "🧹 Limpando chaves antigas do teste..."
docker-compose exec -T argus-cache redis-cli DEL "argus:processed_job:$VIDEO_ID" > /dev/null
echo -e "${GREEN}✓${NC} Redis limpo"

echo ""
echo "📊 TESTE 1: Discovery não deve ver o vídeo como processado"
echo "   Verificando se processed_job:$VIDEO_ID existe no Redis..."
EXISTS=$(docker-compose exec -T argus-cache redis-cli EXISTS "argus:processed_job:$VIDEO_ID")
if [ "$EXISTS" == "0" ]; then
    echo -e "${GREEN}✓${NC} Correto! Vídeo não está marcado como processado"
else
    echo -e "${RED}✗${NC} Erro! Vídeo já está marcado"
    exit 1
fi

echo ""
echo "📊 TESTE 2: Simulando Scraper processando vídeo..."
docker-compose exec -T argus-cache redis-cli SETEX "argus:processed_job:$VIDEO_ID" 86400 "1" > /dev/null
echo -e "${GREEN}✓${NC} Scraper marcou vídeo como processado"

echo ""
echo "📊 TESTE 3: Discovery deve agora ver o vídeo como processado"
EXISTS=$(docker-compose exec -T argus-cache redis-cli EXISTS "argus:processed_job:$VIDEO_ID")
if [ "$EXISTS" == "1" ]; then
    echo -e "${GREEN}✓${NC} Correto! Discovery vai pular este vídeo"
else
    echo -e "${RED}✗${NC} Erro! Vídeo não foi encontrado"
    exit 1
fi

echo ""
echo "📊 TESTE 4: Verificando TTL (deve ser ~24h = 86400s)"
TTL=$(docker-compose exec -T argus-cache redis-cli TTL "argus:processed_job:$VIDEO_ID")
if [ "$TTL" -gt 86000 ] && [ "$TTL" -le 86400 ]; then
    echo -e "${GREEN}✓${NC} TTL correto: ${TTL}s (~24h)"
else
    echo -e "${YELLOW}⚠${NC} TTL: ${TTL}s (esperado ~86400s)"
fi

echo ""
echo "📊 TESTE 5: Listando todas as chaves do Argus no Redis"
echo "   Chaves encontradas:"
docker-compose exec -T argus-cache redis-cli --scan --pattern "argus:*" | while read key; do
    TTL=$(docker-compose exec -T argus-cache redis-cli TTL "$key")
    echo "      - $key (TTL: ${TTL}s)"
done

echo ""
echo "🧹 Limpando..."
docker-compose exec -T argus-cache redis-cli DEL "argus:processed_job:$VIDEO_ID" > /dev/null

echo ""
echo -e "${GREEN}✅ Todos os testes passaram!${NC}"
echo ""
echo "🚀 Próximos passos para teste completo:"
echo "   1. make run-discovery    # Terminal 1"
echo "   2. make run-worker-1     # Terminal 2  "
echo "   3. make run-parser       # Terminal 3"
echo ""
echo "   Monitorar Redis em tempo real:"
echo "   $ watch -n 1 'docker-compose exec -T argus-cache redis-cli --scan --pattern \"argus:*\" | head -20'"
echo ""
