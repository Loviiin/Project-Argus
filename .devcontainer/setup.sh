echo "--- INICIANDO SETUP DO PROJECT ARGUS ---"

echo "Instalando dependências do sistema..."
sudo apt-get update
sudo apt-get install -y libgl1 libglib2.0-0

echo "Configurando Scraper (Node.js)..."
cd services/scraper
npm install
npx playwright install chromium --with-deps
cd ../..

echo "Configurando Vision (Python)..."
cd services/vision
pip install --upgrade pip
pip install -r requirements.txt
cd ../..

echo "--- SETUP CONCLUÍDO! PODE RODAR OS SERVIÇOS ---"
