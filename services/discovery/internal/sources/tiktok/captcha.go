package tiktok

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/nats-io/nats.go"
)

const SadCaptchaBaseURL = "https://www.sadcaptcha.com/api/v1"

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func detectCaptchaType(page *rod.Page) CaptchaType {
	puzzleSelectors := []string{
		".captcha_verify_img_slide",
		"[class*='puzzle']",
		"[class*='slide']",
		".secsdk-captcha-drag-icon",
		"[class*='drag-icon']",
	}

	for _, selector := range puzzleSelectors {
		if _, err := page.Timeout(1 * time.Second).Element(selector); err == nil {
			return CaptchaTypePuzzle
		}
	}

	if _, err := page.Timeout(1*time.Second).ElementR("*", "(?i)(drag.*slider|fit.*puzzle)"); err == nil {
		return CaptchaTypePuzzle
	}
	rotateSelectors := []string{
		"[class*='whirlpool']",
		"[class*='rotate']",
		".captcha_verify_container [class*='outer']",
		".captcha_verify_container [class*='inner']",
	}

	for _, selector := range rotateSelectors {
		if _, err := page.Timeout(1 * time.Second).Element(selector); err == nil {
			return CaptchaTypeRotate
		}
	}

	return CaptchaTypeUnknown
}

func handleRotateCaptcha(page *rod.Page) error {
	fmt.Println(" [Captcha] Detectado captcha de ROTAÇÃO")

	outerB64, innerB64, err := extractRotateImages(page)
	if err != nil {
		return fmt.Errorf("erro extraindo imagens do rotate: %w", err)
	}

	fmt.Println(" [Captcha] Imagens extraídas com sucesso")

	angle, err := solveRotateWithSadCaptcha(outerB64, innerB64)
	if err != nil {
		return fmt.Errorf("erro no SadCaptcha Rotate: %w", err)
	}

	fmt.Printf(" [Captcha] Solução recebida: Ângulo %.2f°\n", angle)

	sliderBar, err := page.Element(".captcha_verify_slide--slidebar")
	if err != nil {
		sliderBar, err = page.Element("[class*='slidebar']")
		if err != nil {
			return fmt.Errorf("slider bar não encontrado")
		}
	}

	sliderIcon, err := page.Element(".secsdk-captcha-drag-icon")
	if err != nil {
		// Tenta seletores alternativos
		sliderIcon, err = page.Element("[class*='drag-icon']")
		if err != nil {
			sliderIcon, err = page.Element("[class*='slide'][class*='btn']")
			if err != nil {
				return fmt.Errorf("slider icon não encontrado")
			}
		}
	}

	// Obtém as dimensões
	barShape, _ := sliderBar.Shape()
	iconShape, _ := sliderIcon.Shape()

	if len(barShape.Quads) == 0 || len(iconShape.Quads) == 0 {
		return fmt.Errorf("não foi possível obter dimensões do slider")
	}

	// Calcula larguras
	l_s := barShape.Quads[0][2] - barShape.Quads[0][0]   // largura da barra
	l_i := iconShape.Quads[0][2] - iconShape.Quads[0][0] // largura do ícone

	// Aplica a fórmula
	pixelsToMove := ((l_s - l_i) * angle) / 360.0

	fmt.Printf("📏 [Captcha] Calculado: %.2f pixels (Barra: %.0fpx, Ícone: %.0fpx)\n",
		pixelsToMove, l_s, l_i)

	// 4. Executa o movimento
	if err := DragSlider(page, sliderIcon, pixelsToMove); err != nil {
		return fmt.Errorf("erro arrastando slider: %w", err)
	}

	fmt.Println(" [Captcha] Slider arrastado com sucesso")
	return nil
}

// handlePuzzleCaptcha resolve captcha do tipo Puzzle usando SadCaptcha
func handlePuzzleCaptcha(page *rod.Page) error {
	fmt.Println(" [Captcha] Detectado captcha de PUZZLE")

	const maxRetries = 5
	const retryDelay = 2 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			fmt.Printf(" [Captcha] Tentativa %d/%d...\n", attempt, maxRetries)
			time.Sleep(retryDelay)
		}

		// 1. Extrai as imagens (Background e Peça)
		bgB64, pieceB64, err := extractPuzzleImages(page)
		if err != nil {
			fmt.Printf("  [Captcha] Erro extraindo imagens (tentativa %d): %v\n", attempt, err)
			continue
		}

		fmt.Println(" [Captcha] Imagens extraídas com sucesso")

		// 2. Tenta resolver usando Vision Service (GRATUITO) primeiro
		var distance float64
		distance, err = solvePuzzleWithVisionService(bgB64, pieceB64)

		if err != nil {
			fmt.Printf("  [Captcha] Vision Service falhou: %v\n", err)
			fmt.Println(" [Captcha] Tentando SadCaptcha como fallback...")

			// Fallback: tenta SadCaptcha (PAGO)
			distance, err = solvePuzzleWithSadCaptcha(bgB64, pieceB64)
			if err != nil {
				fmt.Printf("  [Captcha] Ambos os métodos falharam (tentativa %d)\n", attempt)
				continue
			}
			fmt.Println(" [Captcha] Resolvido com SadCaptcha (fallback)")
		} else {
			fmt.Println(" [Captcha] Resolvido com Vision Service (gratuito)")
		}

		fmt.Printf(" [Captcha] Solução recebida: Distância %.2f pixels\n", distance)

		// Ignora se o offset for 0 (detecção inválida)
		if distance == 0 {
			fmt.Printf("  [Captcha] Offset é 0 - ignorando (tentativa %d)\n", attempt)
			continue
		}

		// 3. Localiza o slider
		slider, err := findSlider(page)
		if err != nil {
			fmt.Printf("  [Captcha] Erro localizando slider (tentativa %d): %v\n", attempt, err)
			continue
		}

		// 4. Executa o movimento
		if err := DragSlider(page, slider, distance); err != nil {
			fmt.Printf("  [Captcha] Erro arrastando slider (tentativa %d): %v\n", attempt, err)
			continue
		}

		fmt.Println(" [Captcha] Slider arrastado com sucesso")

		// Aguarda para ver se o captcha foi resolvido
		time.Sleep(2 * time.Second)

		// Verifica se ainda há captcha (se não houver, sucesso!)
		if !isCaptchaPresent(page) {
			fmt.Printf("🎉 [Captcha] Resolvido com sucesso na tentativa %d!\n", attempt)
			return nil
		}

		fmt.Printf("  [Captcha] Ainda presente após tentativa %d, tentando novamente...\n", attempt)
	}

	return fmt.Errorf("falha ao resolver captcha após %d tentativas", maxRetries)
}

// extractRotateImages extrai as imagens do captcha de rotação em Base64
func extractRotateImages(page *rod.Page) (outer, inner string, err error) {
	// Seletores possíveis para imagens do Rotate
	outerSelectors := []string{
		"[data-testid='whirlpool_outer']",
		"[class*='whirlpool'][class*='outer']",
		".captcha_verify_container img:first-child",
		"[class*='rotate'][class*='outer'] img",
	}

	innerSelectors := []string{
		"[data-testid='whirlpool_inner']",
		"[class*='whirlpool'][class*='inner']",
		".captcha_verify_container img:last-child",
		"[class*='rotate'][class*='inner'] img",
	}

	// Tenta extrair imagem externa
	for _, selector := range outerSelectors {
		if el, err := page.Timeout(2 * time.Second).Element(selector); err == nil {
			if outer, err = extractImageAsBase64(el); err == nil && outer != "" {
				break
			}
		}
	}

	// Tenta extrair imagem interna
	for _, selector := range innerSelectors {
		if el, err := page.Timeout(2 * time.Second).Element(selector); err == nil {
			if inner, err = extractImageAsBase64(el); err == nil && inner != "" {
				break
			}
		}
	}

	if outer == "" || inner == "" {
		return "", "", ErrCaptchaNotFound
	}

	return outer, inner, nil
}

// extractPuzzleImages extrai as imagens do captcha de puzzle em Base64
func extractPuzzleImages(page *rod.Page) (background, piece string, err error) {
	// Usa a função existente extractCaptchaImages
	images, err := extractCaptchaImages(page)
	if err != nil {
		return "", "", err
	}

	// Baixa e converte as URLs para Base64
	background, err = downloadImageAsBase64(images.BackgroundURL)
	if err != nil {
		return "", "", fmt.Errorf("erro baixando background: %w", err)
	}

	piece, err = downloadImageAsBase64(images.PieceURL)
	if err != nil {
		return "", "", fmt.Errorf("erro baixando piece: %w", err)
	}

	return background, piece, nil
}

// extractImageAsBase64 extrai uma imagem de um elemento como Base64
func extractImageAsBase64(el *rod.Element) (string, error) {
	// Tenta obter o recurso binário da imagem
	resource, err := el.Resource()
	if err != nil {
		// Fallback: tenta obter o src como data URL
		src, err := el.Attribute("src")
		if err != nil || src == nil {
			return "", fmt.Errorf("não foi possível obter imagem")
		}

		// Se já é data URL, extrai o base64
		if strings.HasPrefix(*src, "data:image") {
			parts := strings.Split(*src, ",")
			if len(parts) > 1 {
				return parts[1], nil
			}
		}

		// Se é URL, baixa
		return downloadImageAsBase64(*src)
	}

	// Converte para Base64
	return base64.StdEncoding.EncodeToString(resource), nil
}

// downloadImageAsBase64 baixa uma imagem de uma URL e retorna em Base64
func downloadImageAsBase64(imageURL string) (string, error) {
	// Se já é data URL, extrai o base64
	if strings.HasPrefix(imageURL, "data:image") {
		parts := strings.Split(imageURL, ",")
		if len(parts) > 1 {
			return parts[1], nil
		}
	}

	// Baixa a imagem
	resp, err := http.Get(imageURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// solveRotateWithSadCaptcha chama a API do SadCaptcha para resolver Rotate
func solveRotateWithSadCaptcha(outer, inner string) (float64, error) {
	apiKey := os.Getenv("SADCAPTCHA_API_KEY")
	if apiKey == "" {
		return 0, ErrSadCaptchaAPIKey
	}

	url := fmt.Sprintf("%s/rotate?licenseKey=%s", SadCaptchaBaseURL, apiKey)

	payload := SadCaptchaRotateRequest{
		OuterImageB64: outer,
		InnerImageB64: inner,
	}

	jsonPayload, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result SadCaptchaRotateResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("erro parseando resposta: %w", err)
	}

	if result.ErrorID != 0 {
		return 0, fmt.Errorf("%w: %s (ErrorID: %d)", ErrSadCaptchaFailed, result.Message, result.ErrorID)
	}

	return result.Angle, nil
}

// solvePuzzleWithSadCaptcha chama a API do SadCaptcha para resolver Puzzle
func solvePuzzleWithSadCaptcha(background, piece string) (float64, error) {
	apiKey := os.Getenv("SADCAPTCHA_API_KEY")
	if apiKey == "" {
		return 0, ErrSadCaptchaAPIKey
	}

	url := fmt.Sprintf("%s/puzzle?licenseKey=%s", SadCaptchaBaseURL, apiKey)

	payload := SadCaptchaPuzzleRequest{
		PuzzleImageB64: background,
		PieceImageB64:  piece,
	}

	jsonPayload, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result SadCaptchaPuzzleResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("erro parseando resposta: %w", err)
	}

	if result.ErrorID != 0 {
		return 0, fmt.Errorf("%w: %s (ErrorID: %d)", ErrSadCaptchaFailed, result.Message, result.ErrorID)
	}

	return result.Slide, nil
}

// solvePuzzleWithVisionService resolve o puzzle usando o serviço Vision via NATS
// Esta é a alternativa GRATUITA ao SadCaptcha
func solvePuzzleWithVisionService(background, piece string) (float64, error) {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	fmt.Printf(" [NATS] Conectando ao servidor: %s\n", natsURL)

	// Conecta ao NATS
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return 0, fmt.Errorf("erro conectando ao NATS: %w", err)
	}
	defer nc.Close()

	// Prepara payload
	payload := map[string]string{
		"background_b64": background,
		"piece_b64":      piece,
	}

	payloadBytes, _ := json.Marshal(payload)

	fmt.Println(" [NATS] Enviando requisição para jobs.captcha.slider...")

	// Envia requisição e aguarda resposta (request-reply pattern)
	msg, err := nc.Request("jobs.captcha.slider", payloadBytes, 30*time.Second)
	if err != nil {
		return 0, fmt.Errorf("erro na requisição NATS: %w", err)
	}

	// DEBUG: Log da resposta bruta
	fmt.Printf(" [NATS] Resposta bruta: %s\n", string(msg.Data))

	// Parse resposta
	var response struct {
		XOffset    float64 `json:"x_offset"`
		Success    bool    `json:"success"`
		Confidence float64 `json:"confidence"`
		Error      string  `json:"error"`
	}

	if err := json.Unmarshal(msg.Data, &response); err != nil {
		return 0, fmt.Errorf("erro parseando resposta: %w", err)
	}

	// DEBUG: Log do struct parseado
	fmt.Printf(" [NATS] Struct: success=%v, x_offset=%.2f, confidence=%.4f, error='%s'\n",
		response.Success, response.XOffset, response.Confidence, response.Error)

	if !response.Success {
		return 0, fmt.Errorf("Vision Service falhou: %s", response.Error)
	}

	fmt.Printf(" [NATS] Resposta recebida: x_offset = %.2f (confiança: %.2f%%)\n",
		response.XOffset, response.Confidence*100)

	// Avisa se a confiança é muito baixa
	if response.Confidence < 0.3 {
		fmt.Printf("  [NATS] Confiança baixa (%.1f%%). Resultado pode não ser preciso.\n",
			response.Confidence*100)
	}

	return response.XOffset, nil
}

// extractCaptchaImages detecta e extrai as URLs das imagens do captcha
// Retorna a URL da imagem de fundo e da peça do quebra-cabeça
func extractCaptchaImages(page *rod.Page) (*CaptchaImages, error) {
	fmt.Println("🔍 [Captcha] Procurando elementos de imagem...")

	// DEBUG: Lista TODOS os elementos visíveis na página
	fmt.Println(" [Debug] Listando estrutura do DOM...")
	debugElements(page)

	// Estratégia 1: Procurar por iframe do captcha
	iframe, err := page.Timeout(3 * time.Second).Element(`iframe[src*="captcha"]`)
	if err == nil {
		fmt.Println(" [Captcha] Iframe de captcha encontrado")
		// Se encontrou iframe, entra nele
		page = iframe.MustFrame()
		fmt.Println(" [Debug] Listando estrutura do iframe...")
		debugElements(page)
	}

	// Estratégia 2: Buscar pelos seletores comuns do TikTok
	var backgroundURL, pieceURL string

	// Tenta encontrar a imagem de fundo (background)
	bgSelectors := []string{
		`img[alt="Captcha"]:first-of-type`,
		`img[class*="cap-h"]`,
		`img[class*="captcha_verify_img"]`,
		`img[class*="captcha"][class*="bg"]`,
		`img[class*="verify"][class*="background"]`,
		`.captcha_verify_img_slide > img:first-child`,
		`.captcha_verify_img > img`,
		`div[class*="captcha"] img[src*="captcha"]`,
		`div[class*="verify"] img`,
	}

	for _, selector := range bgSelectors {
		if el, err := page.Timeout(1 * time.Second).Element(selector); err == nil {
			if src, err := el.Attribute("src"); err == nil && src != nil {
				backgroundURL = *src
				fmt.Printf(" [Captcha] Background encontrado via: %s\n", selector)
				break
			}
		}
	}

	// Tenta encontrar a peça do quebra-cabeça (puzzle piece)
	pieceSelectors := []string{
		`img[class*="cap-absolute"]`,
		`img[alt="Captcha"]:last-of-type`,
		`img[class*="captcha_verify_slide_img"]`,
		`img[class*="captcha"][class*="piece"]`,
		`img[class*="verify"][class*="puzzle"]`,
		`.captcha_verify_img_slide > img:last-child`,
		`div[class*="slide_block"] img`,
		`div[class*="captcha_verify_slide"] img`,
	}

	for _, selector := range pieceSelectors {
		if el, err := page.Timeout(1 * time.Second).Element(selector); err == nil {
			if src, err := el.Attribute("src"); err == nil && src != nil {
				pieceURL = *src
				fmt.Printf(" [Captcha] Piece encontrado via: %s\n", selector)
				break
			}
		}
	}

	// Estratégia 3: Buscar por elementos canvas (alguns captchas renderizam em canvas)
	if backgroundURL == "" || pieceURL == "" {
		fmt.Println("🔍 [Captcha] Tentando extrair de canvas...")
		canvases, _ := page.Elements("canvas")
		for i, canvas := range canvases {
			// Tenta extrair o conteúdo do canvas como data URL
			dataURL, err := canvas.Evaluate(&rod.EvalOptions{
				JS: `() => this.toDataURL('image/png')`,
			})
			if err == nil && dataURL.Value.String() != "" {
				url := dataURL.Value.String()
				if i == 0 && backgroundURL == "" {
					backgroundURL = url
					fmt.Println(" [Captcha] Background extraído de canvas")
				} else if i == 1 && pieceURL == "" {
					pieceURL = url
					fmt.Println(" [Captcha] Piece extraído de canvas")
				}
			}
		}
	}

	// Estratégia 4: Buscar todas as imagens no container de captcha
	if backgroundURL == "" || pieceURL == "" {
		fmt.Println("🔍 [Captcha] Buscando imagens no container...")

		// Tenta containers possíveis
		containerSelectors := []string{
			"[class*='captcha-verify']",
			"[class*='captcha_verify']",
			"[class*='TUXModal']",
		}

		for _, containerSel := range containerSelectors {
			captchaContainer, err := page.Timeout(2 * time.Second).Element(containerSel)
			if err == nil {
				images, _ := captchaContainer.Elements("img")
				fmt.Printf(" [Captcha] Encontradas %d imagens no container '%s'\n", len(images), containerSel)

				for i, img := range images {
					if src, err := img.Attribute("src"); err == nil && src != nil && *src != "" {
						alt, _ := img.Attribute("alt")
						altStr := ""
						if alt != nil {
							altStr = *alt
						}

						srcPreview := (*src)[:min(80, len(*src))]
						fmt.Printf("  Imagem %d (alt='%s'): %s...\n", i+1, altStr, srcPreview)

						if i == 0 && backgroundURL == "" {
							backgroundURL = *src
						} else if i == 1 && pieceURL == "" {
							pieceURL = *src
						}

						// Se já pegou as 2, para
						if backgroundURL != "" && pieceURL != "" {
							break
						}
					}
				}

				// Se encontrou no container, para de procurar
				if backgroundURL != "" && pieceURL != "" {
					break
				}
			}
		}
	}

	// Estratégia 5: Fallback - procurar por qualquer imagem com 'captcha' na URL
	if backgroundURL == "" || pieceURL == "" {
		fmt.Println("🔍 [Captcha] Fallback: buscando todas as imagens...")
		images, _ := page.Elements("img")
		count := 0
		for _, img := range images {
			if src, err := img.Attribute("src"); err == nil && src != nil && *src != "" {
				if strings.Contains(strings.ToLower(*src), "captcha") ||
					strings.Contains(strings.ToLower(*src), "verify") {
					count++
					fmt.Printf("  Imagem captcha %d encontrada\n", count)
					if backgroundURL == "" {
						backgroundURL = *src
					} else if pieceURL == "" {
						pieceURL = *src
						break
					}
				}
			}
		}
	}

	if backgroundURL == "" || pieceURL == "" {
		fmt.Printf("❌ [Captcha] Extração falhou - BG: %v, Piece: %v\n",
			backgroundURL != "", pieceURL != "")
		return nil, ErrCaptchaNotFound
	}

	fmt.Println(" [Captcha] Ambas as imagens extraídas com sucesso")
	return &CaptchaImages{
		BackgroundURL: backgroundURL,
		PieceURL:      pieceURL,
	}, nil
}

// findSlider localiza o elemento do slider que deve ser arrastado
func findSlider(page *rod.Page) (*rod.Element, error) {
	// Seletores específicos do TikTok (baseado no debug)
	sliderSelectors := []string{
		// TikTok específico
		`button[class*="secsdk-captcha-drag-icon"]`,
		`[class*="secsdk-captcha-drag-icon"]`,
		`button[class*="TUXButton"][class*="drag"]`,
		// Genéricos
		`div[class*="slide"][class*="btn"]`,
		`div[class*="slider"][class*="button"]`,
		`.captcha_verify_slide > div`,
		`div[id*="slide"][id*="block"]`,
		`span[class*="slide"][class*="move"]`,
		`div[class*="secsdk-captcha-drag"]`,
		// Fallback: qualquer botão dentro do container de captcha
		`.captcha-verify-container button`,
		`[class*="captcha-verify"] button`,
	}

	fmt.Println("🔍 [Captcha] Procurando elemento slider...")

	for _, selector := range sliderSelectors {
		if el, err := page.Timeout(2 * time.Second).Element(selector); err == nil {
			// Verifica se o elemento está visível
			visible, _ := el.Visible()
			if visible {
				fmt.Printf(" [Captcha] Slider encontrado: %s\n", selector)
				return el, nil
			} else {
				fmt.Printf("  [Captcha] Slider encontrado mas invisível: %s\n", selector)
			}
		}
	}

	// DEBUG: Lista todos os botões na página
	fmt.Println(" [Debug] Listando todos os botões...")
	buttons, _ := page.Elements("button")
	for i, btn := range buttons {
		if i >= 5 {
			break
		}
		class, _ := btn.Attribute("class")
		classStr := ""
		if class != nil {
			classStr = *class
		}
		fmt.Printf("  Button[%d]: class='%s'\n", i+1, classStr)
	}

	return nil, fmt.Errorf("slider não encontrado na página")
}

// debugElements lista os elementos do DOM para debug
func debugElements(page *rod.Page) {
	// Lista divs com classe contendo 'captcha' ou 'verify'
	captchaDivs := []string{"[class*='captcha']", "[class*='verify']", "[class*='secsdk']"}

	for _, selector := range captchaDivs {
		elements, _ := page.Elements(selector)
		if len(elements) > 0 {
			fmt.Printf("🔍 Encontrados %d elementos com selector '%s'\n", len(elements), selector)
		}
		for i, el := range elements {
			if i >= 5 {
				break // Limita para não poluir
			}
			class, _ := el.Attribute("class")
			classStr := ""
			if class != nil {
				classStr = *class
			}
			fmt.Printf("  [%d] class='%s'\n", i+1, classStr)
		}
	}

	// Lista todas as imagens
	images, _ := page.Elements("img")
	fmt.Printf("🖼️  Total de imagens na página: %d\n", len(images))
	for i, img := range images {
		if i >= 5 {
			fmt.Printf("   ... e mais %d imagens\n", len(images)-5)
			break
		}
		src, _ := img.Attribute("src")
		alt, _ := img.Attribute("alt")
		class, _ := img.Attribute("class")

		srcStr, altStr, classStr := "", "", ""
		if src != nil {
			srcStr = *src
			if len(srcStr) > 80 {
				srcStr = srcStr[:80] + "..."
			}
		}
		if alt != nil {
			altStr = *alt
		}
		if class != nil {
			classStr = *class
		}

		fmt.Printf("  IMG[%d]: class='%s' alt='%s' src='%s'\n", i+1, classStr, altStr, srcStr)
	}
}

// waitCaptchaResolution aguarda até que o CAPTCHA seja resolvido manualmente
// ou até que o tempo limite seja atingido
func waitCaptchaResolution(page *rod.Page, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	fmt.Printf("[Captcha] Aguardando resolução manual. Acesse http://localhost:9222\n")
	fmt.Printf("[Captcha] Tempo limite: %s\n", maxWait)

	for time.Now().Before(deadline) {
		if !isCaptchaPresent(page) {
			fmt.Println("[Captcha] Resolvido manualmente!")
			return nil
		}
		time.Sleep(2 * time.Second)
	}

	return ErrCaptchaTimeout
}
