# Pacote TikTok - Scraper Modular

Este pacote contém a implementação modular do scraper do TikTok usando Go-Rod.

## 📁 Estrutura de Arquivos

```
tiktok/
├── client.go        # Navegação e orquestração principal
├── captcha.go       # Detecção e extração de captcha
├── mouse.go         # Movimento humanizado do mouse (Curvas de Bézier)
├── nats_solver.go   # Comunicação NATS (Stub para integração futura)
└── types.go         # Structs e tipos de dados
```

## 🎯 Responsabilidades

### client.go

- Inicialização do browser com Stealth Mode
- Navegação em páginas de tags e vídeos
- Interceptação de APIs (HijackRequests)
- Orquestração do fluxo de scraping
- Gerenciamento de timeouts e retries

### captcha.go

- Detecção de páginas de captcha
- Extração de URLs de imagens (background + piece)
- Localização do elemento slider
- Múltiplas estratégias de detecção (iframe, seletores CSS, canvas)

### mouse.go

- Movimento humanizado usando **Curvas de Bézier Cúbicas**
- Aceleração e desaceleração variável
- Tremor humano (micro-movimentos aleatórios)
- Micro-pausas durante o arrasto
- Função `DragSlider()` para resolver captchas de slider

### nats_solver.go

- **Stub** para integração futura com NATS JetStream
- Mock que simula resposta do serviço Vision
- Documentação completa do protocolo de comunicação
- Estrutura preparada para troca de mensagens

### types.go

- `RawVideoMetadata` - Metadados de vídeo
- `TikTokAPIResponse` - Resposta da API interna
- `CaptchaImages` - URLs das imagens do captcha
- `CaptchaSolution` - Resposta do solver
- Erros customizados

## 🚀 Uso

```go
import "discovery/internal/sources/tiktok"

// Criar instância
source := tiktok.NewSource()

// Buscar por tag
results, err := source.Fetch("viralvideos")

// Buscar URL direta
results, err := source.Fetch("https://www.tiktok.com/@user/video/123")
```

## 🧩 Fluxo de Resolução de Captcha

### Tipos Suportados

O sistema detecta e resolve automaticamente dois tipos de captcha:

#### 1. 🔄 Rotate (Rotação)

Alinhar círculos girando a imagem.

```
1. Navegação → Detecta Captcha (captcha.go)
                    ↓
2. detectCaptchaType() → CaptchaTypeRotate
                    ↓
3. extractRotateImages() → outer.png + inner.png (Base64)
                    ↓
4. solveRotateWithSadCaptcha() → angle: 245.7°
                    ↓
5. Fórmula: pixels = ((largura_barra - largura_icone) * angle) / 360
                    ↓
6. DragSlider() → Movimento humanizado (mouse.go)
                    ↓
7. Valida resolução → ✅ Sucesso
```

#### 2. 🧩 Puzzle (Quebra-cabeça)

Encaixar a peça no buraco.

```
1. Navegação → Detecta Captcha (captcha.go)
                    ↓
2. detectCaptchaType() → CaptchaTypePuzzle
                    ↓
3. extractPuzzleImages() → background.png + piece.png (Base64)
                    ↓
4. solvePuzzleWithSadCaptcha() → slide: 152px
                    ↓
5. DragSlider() → Movimento humanizado (mouse.go)
                    ↓
6. Valida resolução → ✅ Sucesso
```

### Configuração SadCaptcha

```bash
# Variável de ambiente necessária
export SADCAPTCHA_API_KEY="sua_api_key_aqui"
```

Para detalhes completos, veja [SADCAPTCHA_CONFIG.md](../SADCAPTCHA_CONFIG.md).

### Fórmula Matemática (Rotate)

A conversão de ângulo para pixels usa:

```
d = ((l_s - l_i) * a) / 360

Onde:
- d  = distância em pixels
- l_s = largura da barra de slide
- l_i = largura do ícone (botão)
- a  = ângulo retornado pela API (0-360°)
```

## 🔧 TODO - Integração NATS

Para ativar a integração com NATS, siga o guia em `nats_solver.go`:

1. Adicionar dependências:

   ```bash
   go get github.com/nats-io/nats.go
   go get github.com/nats-io/nats.go/jetstream
   ```

2. Descomentar o código em `NewCaptchaSolver()` e `RequestSolution()`

3. Configurar URL do NATS (variável de ambiente ou config)

4. Definir tópicos:
   - Publicação: `jobs.captcha.solve`
   - Resposta: `jobs.captcha.result.<request_id>`

5. Implementar retry com backoff exponencial

## 🎨 Curvas de Bézier

O movimento do mouse usa curvas de Bézier cúbicas para simular movimento humano:

```
P(t) = (1-t)³P₀ + 3(1-t)²tP₁ + 3(1-t)t²P₂ + t³P₃
```

Onde:

- P₀ = Ponto inicial
- P₁, P₂ = Pontos de controle (aleatórios)
- P₃ = Ponto final
- t ∈ [0, 1]

Características:

- ✅ Aceleração no início
- ✅ Velocidade constante no meio
- ✅ Desaceleração no final
- ✅ Tremor aleatório (+/- 2px)
- ✅ Micro-pausas ocasionais

## 🔍 Seletores CSS Comuns

### Imagem de Fundo

```css
img[class*="captcha"][class*="bg"]
img[class*="verify"][class*="background"]
.captcha_verify_img_slide > img:first-child
```

### Peça do Quebra-Cabeça

```css
img[class*="captcha"][class*="piece"]
div[class*="slide_block"] img
.captcha_verify_img_slide > img:last-child
```

### Slider

```css
div[class*="slide"][class*="btn"]
div[class*="slider"][class*="button"]
div[class*="secsdk-captcha-drag"]
```

## 📊 Timeouts

- `fetchTimeout`: 45s - Coleta de vídeos na tag
- `perVideoTimeout`: 20s - Processamento individual de vídeo
- `captchaWaitLimit`: 60s - Tempo máximo para resolver captcha

## 🐛 Debugging

O Go-Rod DevTools está disponível em:

```
http://localhost:9222
```

Use para:

- Inspecionar páginas em tempo real
- Debugar seletores CSS
- Ver logs de rede
- Resolver captchas manualmente (fallback)

## 🔒 Anti-Bot Features

1. **Stealth Mode** - Mascara o Rod/Chromium
2. **Movimento Humanizado** - Curvas de Bézier + ruído
3. **Delays Aleatórios** - Entre 100-300ms
4. **User-Agent Real** - Via Stealth
5. **Scroll Gradual** - Simula leitura
6. **Interceptação de API** - Não usa DOM parsing

## 📝 Logs

Prefixos usados:

- `[Rod]` - Navegação e scraping geral
- `[Captcha]` - Detecção e resolução de captcha
- `[Mouse]` - Movimento e arrasto
- `[NATS]` - Comunicação com Vision service

## 🧪 Testing

TODO: Criar testes unitários para:

- [ ] Extração de IDs de URL
- [ ] Detecção de captcha
- [ ] Cálculo de curvas de Bézier
- [ ] Parsing de respostas da API
- [ ] Mock de NATS

## 📚 Referências

- [Go-Rod Documentation](https://go-rod.github.io/)
- [Bézier Curves](https://en.wikipedia.org/wiki/B%C3%A9zier_curve)
- [NATS JetStream](https://docs.nats.io/nats-concepts/jetstream)
