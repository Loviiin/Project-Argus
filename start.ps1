param()

Write-Host "=========================================" -ForegroundColor Cyan
Write-Host "   Inicializando Project-Argus (Docker)  " -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan

# 1. Verifica se o Docker esta rodando
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Host "ERRO: Docker nao esta instalado ou nao esta no PATH. Instale o Docker Desktop." -ForegroundColor Red
    Pause
    exit 1
}

# 2. Configura o config.yaml
$configPath = "config\config.yaml"
$examplePath = "config\config.example.yaml"

if (-not (Test-Path $configPath)) {
    Write-Host "Arquivo config.yaml nao encontrado. Criando um a partir do template..." -ForegroundColor Yellow
    Copy-Item $examplePath $configPath
}

Write-Host "`nPor favor, garanta que o seu ttwid esta preenchido no arquivo config/config.yaml antes de prosseguir." -ForegroundColor Yellow
Write-Host "Pressione qualquer tecla quando estiver pronto para iniciar os containers..." -ForegroundColor Cyan
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")

# 3. Corrige os localhost para os nomes dos containers
Write-Host "`nAjustando o config.yaml para a rede interna do Docker Compose..." -ForegroundColor Yellow
$content = Get-Content $configPath -Raw
$content = $content -replace '127\.0\.0\.1:5432', 'argus-db:5432'
$content = $content -replace 'localhost:5432', 'argus-db:5432'
$content = $content -replace 'localhost:7700', 'argus-meili:7700'
$content = $content -replace 'localhost:6379', 'argus-cache:6379'
$content = $content -replace 'localhost:4222', 'nats-jetstream:4222'
$content = $content -replace 'localhost:8080', 'tiktok-signer:8080'
Set-Content -Path $configPath -Value $content

# 4. Sobe o ambiente
Write-Host "`nCompilando as imagens Go e subindo toda a infraestrutura em background..." -ForegroundColor Green
docker compose up -d --build

if ($LASTEXITCODE -eq 0) {
    Write-Host "`nSucesso! Todos os servicos estao rodando." -ForegroundColor Green
    Write-Host "Para visualizar os servidores, acesse o Meilisearch em: http://localhost:7700" -ForegroundColor Cyan
    Write-Host "Para ver os logs dos trabalhadores: docker compose logs -f argus-discovery argus-scraper argus-parser" -ForegroundColor Gray
} else {
    Write-Host "`nHouve um erro ao tentar subir os containers." -ForegroundColor Red
}

Pause
