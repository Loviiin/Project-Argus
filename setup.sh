#!/bin/bash
set -e

echo "=== 🛠️  Atualizando Sistema ==="
sudo apt update && sudo apt upgrade -y
sudo apt install -y wget curl git make build-essential libgl1 libglib2.0-0

echo "=== 🖥️  Instalando Desktop + VNC + noVNC ==="
# Desktop leve para o Chrome ter display
sudo apt install -y xfce4 xfce4-goodies

# VNC Server
sudo apt install -y tightvncserver

# noVNC para acesso pelo browser
sudo apt install -y novnc websockify

# Configura VNC (vai pedir senha na primeira vez)
if [ ! -f ~/.vnc/passwd ]; then
    echo "⚠️  Configure a senha do VNC:"
    vncserver :1 -geometry 1280x800 -depth 24
    vncserver -kill :1
fi

# Cria script de inicialização do VNC
mkdir -p ~/.vnc
cat > ~/.vnc/xstartup << 'EOF'
#!/bin/bash
xrdb $HOME/.Xresources
startxfce4 &
EOF
chmod +x ~/.vnc/xstartup

# Cria serviço systemd para VNC
sudo tee /etc/systemd/system/vncserver.service > /dev/null << EOF
[Unit]
Description=VNC Server
After=network.target

[Service]
Type=forking
User=$USER
ExecStart=/usr/bin/vncserver :1 -geometry 1280x800 -depth 24
ExecStop=/usr/bin/vncserver -kill :1
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

# Cria serviço systemd para noVNC
sudo tee /etc/systemd/system/novnc.service > /dev/null << EOF
[Unit]
Description=noVNC Web Client
After=vncserver.service

[Service]
Type=simple
User=$USER
ExecStart=/usr/bin/websockify --web=/usr/share/novnc 6080 localhost:5901
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable vncserver novnc
sudo systemctl start vncserver novnc

echo "=== 🔥 Configurando Firewall ==="
sudo ufw allow 22
sudo ufw allow 6080
sudo ufw --force enable

echo "=== 🐹 Instalando Go ==="
sudo rm -rf /usr/local/go
wget https://go.dev/dl/go1.26.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.26.0.linux-amd64.tar.gz
rm go1.26.0.linux-amd64.tar.gz

if ! grep -q "/usr/local/go/bin" ~/.bashrc; then
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
fi
export PATH=$PATH:/usr/local/go/bin

echo "=== 🌐 Instalando Google Chrome ==="
wget https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
sudo apt install -y ./google-chrome-stable_current_amd64.deb
rm google-chrome-stable_current_amd64.deb

echo "=== 🐍 Instalando Python & Dependências de ML ==="
sudo apt install -y python3 python3-pip python3-venv

if [ ! -d "services/vision/venv" ]; then
    echo "Criando VENV em services/vision/venv..."
    python3 -m venv services/vision/venv
fi

source services/vision/venv/bin/activate
pip install --upgrade pip
pip install -r services/vision/requirements.txt
deactivate

echo "=== 🐳 Instalando Docker ==="
if ! command -v docker &> /dev/null; then
    sudo apt install -y docker.io docker-compose-v2
    sudo usermod -aG docker $USER
else
    echo "Docker já instalado."
fi

echo ""
echo "════════════════════════════════════════════"
echo "✅ Setup Concluído!"
echo ""
echo "Acesse o VNC pelo browser:"
echo "  http://$(hostname -I | awk '{print $1}'):6080/vnc.html"
echo ""
echo "⚠️  Rode antes de usar:"
echo "  source ~/.bashrc"
echo "  newgrp docker  # para usar Docker sem sudo"
echo "════════════════════════════════════════════"