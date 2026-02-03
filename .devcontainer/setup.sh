echo "--- INICIANDO SETUP DO PROJECT ARGUS ---"

echo "Instalando dependências do sistema..."

echo "Configurando Vision (Python)..."
cd services/vision
pip install --upgrade pip
pip install -r requirements.txt
cd ../..

echo "--- SETUP CONCLUÍDO! PODE RODAR OS SERVIÇOS ---"
