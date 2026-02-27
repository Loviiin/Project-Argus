#!/bin/bash
set -e

echo "=== 🛠️  Atualizando Sistema ==="
sudo apt update && sudo apt upgrade -y
# Instala dependências básicas de compilação e bibliotecas gráficas pro OpenCV/PyTorch
sudo apt install -y wget curl git make build-essential libgl1 libglib2.0-0

echo "=== 🐹 Instalando Go 1.26 (Versão Recente) ==="
# Limpa instalações antigas
sudo rm -rf /usr/local/go
wget https://go.dev/dl/go1.26.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.26.0.linux-amd64.tar.gz
rm go1.26.0.linux-amd64.tar.gz

# Configura PATH (Adiciona no .bashrc se não existir)
if ! grep -q "/usr/local/go/bin" ~/.bashrc; then
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
fi

# Exporta temporariamente para usar neste script
export PATH=$PATH:/usr/local/go/bin

echo "=== 🌐 Instalando Google Chrome (Para o Scraper) ==="
wget https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
sudo apt install -y ./google-chrome-stable_current_amd64.deb
rm google-chrome-stable_current_amd64.deb

echo "=== 🐍 Instalando Python & Dependências de ML ==="
sudo apt install -y python3 python3-pip python3-venv

# Cria o ambiente virtual se não existir
if [ ! -d "services/vision/venv" ]; then
    echo "Criando VENV em services/vision/venv..."
    python3 -m venv services/vision/venv
fi

# Ativa e instala requirements
source services/vision/venv/bin/activate
echo "Instalando bibliotecas Python (PyTorch, Numpy, etc)..."
# Garante que o pip está atualizado
pip install --upgrade pip
# Instala as dependências do projeto
pip install -r services/vision/requirements.txt
deactivate

echo "=== 🐳 Instalando Docker (Apenas para Infra: Redis/Postgres/NATS) ==="
if ! command -v docker &> /dev/null; then
    sudo apt install -y docker.io docker-compose-v2
    sudo usermod -aG docker $USER
else
    echo "Docker já instalado."
fi

echo "=== ✅ Setup Concluído! ==="
echo "Por favor, rode: source ~/.bashrc"