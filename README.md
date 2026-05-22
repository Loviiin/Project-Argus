# Project Argus

**OSINT Pipeline: Descobrindo Servidores Discord via TikTok**

O Project-Argus é um sistema de OSINT (Open Source Intelligence) contínuo e escalável projetado para varrer o TikTok em busca de convites e servidores do Discord, validando, enriquecendo e indexando esses dados para análise avançada.

---

## 🏗️ Arquitetura

O Argus foi redesenhado do zero para ser uma **Arquitetura Distribuída**, dividida em microsserviços autônomos que se comunicam através de filas rápidas.

### Serviços Principais (Go)
1. **Discovery**: Monitora hashtags específicas (ex: `#discordserver`) utilizando paginação orgânica. Assim que encontra vídeos interessantes, joga na fila.
2. **Scraper**: Pega os vídeos da fila, extrai todos os comentários e armazena os dados brutos no PostgreSQL.
3. **Parser**: Consome os comentários brutos, aplica RegEx pesados e NLP para descobrir links de `discord.gg`. Ao encontrar, chama a API do Discord, pega os dados do servidor (Membros, Nome, Foto) e indexa tudo.

### Infraestrutura & Cache
- **NATS Jetstream**: Fila de mensagens ultrarrápida que conecta os 3 serviços.
- **Redis**: Controla bloqueios de concorrência (`processing_lock`) e dedup (eliminação de links/vídeos duplicados).
- **PostgreSQL**: Banco de dados relacional para armazenamento de longo prazo (comentários brutos, metadata).
- **Meilisearch**: Motor de busca e Painel Visual para varrer e filtrar os convites descobertos.

### O "Cheat-Code": Sidecar
Para burlar os rigorosos *Shadowbans*, *Captchas* e o temido "Response 0-byte" da API do TikTok, o Argus usa um **Sidecar** (Node.js/Puppeteer).
O Sidecar é acionado via API interna para gerar assinaturas complexas (`X-Bogus`, `X-Gnarly`) nativamente. Caso uma requisição direta falhe, os workers pedem para o Sidecar injetar o *Cookie* (ttwid) e utilizar a rota de `/fetch` do navegador headless como fallback garantido.

---

## 🚀 Como Rodar (Plug & Play)

Todo o ecossistema foi dockerizado (Bancos, Filas, Sidecar e Workers Go). O setup é extremamente simples e não requer nada além do **Docker Desktop** instalado.

### 1. Clonar e Configurar
```bash
git clone https://github.com/Loviiin/Project-Argus.git
cd Project-Argus
```

### 2. Rodar!
Nós criamos scripts amigáveis que checam dependências, preparam seus arquivos de configuração e sobem todo o cluster.

#### No Windows (PowerShell):
```powershell
.\start.ps1
```

#### No Linux / Ubuntu Server:
```bash
chmod +x start.sh
./start.sh
```

O script perguntará pelo seu `ttwid` (Cookie do TikTok) na primeira vez, ajustará os IPs para a rede interna do Docker e fará o build de tudo.

---

## 📊 Acesso e Painéis

Quando os scripts avisarem que tudo subiu, a infraestrutura local estará viva.

- **Painel de Busca (Meilisearch):** `http://localhost:7700`
- **PostgreSQL:** `localhost:5432` (Usuário: `argus-user`, Senha: `change_me`, Banco: `argus-post-db`)
- **Redis:** `localhost:6379`
- **Logs ao vivo:**
  ```bash
  docker compose logs -f argus-discovery argus-scraper argus-parser
  ```

---

## 💻 Cluster Multi-Node (Avançado)
Como os workers conversam através do **NATS** e **Redis**, você pode instalar o Argus em vários PCs ou Servidores VPS ao mesmo tempo. 

Basta usar a sua máquina principal como **Control Plane** (Rodando o Banco de dados e NATS) e configurar o `config.yaml` dos outros PCs para apontar para o IP local/remoto da sua máquina principal. Assim, múltiplos *Scrapers* estarão varrendo o TikTok simultaneamente e enchendo o mesmo banco de dados.
