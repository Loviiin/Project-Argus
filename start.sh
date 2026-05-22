#!/bin/bash
set -e

echo "========================================="
echo "   Inicializando Project-Argus (Docker)  "
echo "========================================="

# 1. Verifica se o Docker esta rodando
if ! command -v docker &> /dev/null; then
    echo "❌ ERRO: Docker nao esta instalado. Rode o setup.sh primeiro."
    exit 1
fi

# 2. Configura o config.yaml
CONFIG_PATH="config/config.yaml"
EXAMPLE_PATH="config/config.example.yaml"

if [ ! -f "$CONFIG_PATH" ]; then
    echo "⚠️  Arquivo config.yaml nao encontrado. Criando um a partir do template..."
    cp "$EXAMPLE_PATH" "$CONFIG_PATH"
fi

echo -e "\n⚠️  Por favor, garanta que o seu ttwid esta preenchido no arquivo config/config.yaml antes de prosseguir."
read -p "Pressione [Enter] quando estiver pronto para iniciar os containers..."

# 3. Corrige os localhost para os nomes dos containers
echo -e "\n🔄 Ajustando o config.yaml para a rede interna do Docker Compose..."
sed -i 's/127\.0\.0\.1:5432/argus-db:5432/g' "$CONFIG_PATH"
sed -i 's/localhost:5432/argus-db:5432/g' "$CONFIG_PATH"
sed -i 's/localhost:7700/argus-meili:7700/g' "$CONFIG_PATH"
sed -i 's/localhost:6379/argus-cache:6379/g' "$CONFIG_PATH"
sed -i 's/localhost:4222/nats-jetstream:4222/g' "$CONFIG_PATH"
sed -i 's/localhost:8080/tiktok-signer:8080/g' "$CONFIG_PATH"

# 4. Sobe o ambiente
echo -e "\n🚀 Compilando as imagens Go e subindo toda a infraestrutura em background..."
docker compose up -d --build

echo -e "\n✅ Sucesso! Todos os servicos estao rodando."
echo "Para visualizar os servidores pelo PC do Ryzen, acesse o IP deste Pentium na porta 7700 (Ex: http://192.168.x.x:7700)"
echo "Para ver os logs: docker compose logs -f argus-discovery argus-scraper argus-parser"
