echo "--- INICIANDO SETUP DO PROJECT ARGUS ---"

echo "Instalando dependências do sistema..."

echo "Configurando Vision (Python)..."
cd services/vision
pip install --upgrade pip
pip install torch torchvision --index-url https://download.pytorch.org/whl/cpu
pip install -r requirements.txt
cd ../..

echo ""
echo "--- SETUP CONCLUÍDO! PODE RODAR OS SERVIÇOS ---"
echo ""
echo "🖥️  noVNC (acesso visual ao browser):"
echo "   O display virtual sobe automaticamente ao abrir o container."
echo "   Acesse http://localhost:6080 no seu browser Windows e clique em 'Connect'."
echo "   O Chromium do Discovery aparecerá lá quando você rodar: make run-discovery"
echo ""
