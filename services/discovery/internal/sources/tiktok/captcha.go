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

// min retorna o menor entre dois inteiros
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// debugElements lista elementos visíveis da página para debug
func debugElements(page *rod.Page) {
	elements, err := page.Elements("*")
	if err != nil {
		fmt.Printf("⚠️  [Debug] Erro listando elementos: %v\n", err)
		return
	}

	fmt.Printf("🐛 [Debug] Total de elementos: %d\n", len(elements))

	// Lista elementos relevantes para captcha
	for i, el := range elements {
		class, _ := el.Attribute("class")
		id, _ := el.Attribute("id")
		alt, _ := el.Attribute("alt")
		tag, _ := el.Evaluate(&rod.EvalOptions{JS: `() => this.tagName`})

		tagName := ""
		if tag != nil {
			tagName = tag.Value.String()
		}

		classStr := ""
		if class != nil {
			classStr = *class
		}

		idStr := ""
		if id != nil {
			idStr = *id
		}

		altStr := ""
		if alt != nil {
			altStr = *alt
		}

		// Filtra para mostrar apenas elementos relevantes
		if strings.Contains(strings.ToLower(classStr), "captcha") ||
			strings.Contains(strings.ToLower(classStr), "slide") ||
			strings.Contains(strings.ToLower(classStr), "verify") ||
			strings.Contains(strings.ToLower(classStr), "secsdk") ||
			strings.Contains(strings.ToLower(idStr), "captcha") ||
			tagName == "IMG" || tagName == "CANVAS" || tagName == "BUTTON" {
			fmt.Printf("  [%d] <%s> class='%s' id='%s' alt='%s'\n", i, tagName, classStr, idStr, altStr)
		}
	}
}

// detectCaptchaType identifica qual tipo de captcha está presente na página
// Por padrão, assume ROTATE já que é o mais comum no TikTok atualmente
func detectCaptchaType(page *rod.Page) CaptchaType {
	// O TikTok está usando principalmente captchas de rotação
	// Só detecta como Puzzle se houver evidência muito clara

	// Verifica por texto "Drag the slider to fit the puzzle" que é específico do Puzzle
	if _, err := page.Timeout(500*time.Millisecond).ElementR("*", "(?i)(fit.*puzzle|encaixe.*peça)"); err == nil {
		return CaptchaTypePuzzle
	}

	// Por padrão, assume ROTATE
	return CaptchaTypeRotate
}

// handleRotateCaptcha resolve captcha do tipo Rotate usando Vision Service
// A rotação é controlada por um slider horizontal
func handleRotateCaptcha(page *rod.Page) error {
	fmt.Println("🔄 [Captcha] Detectado captcha de ROTAÇÃO")

	const maxRetries = 5
	const retryDelay = 2 * time.Second

	var lastOuterB64, lastInnerB64 string

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			fmt.Printf("🔄 [Captcha] Tentativa %d/%d...\n", attempt, maxRetries)
			time.Sleep(retryDelay)
		}

		outerB64, innerB64, err := extractRotateImages(page)
		if err != nil {
			fmt.Printf("⚠️  [Captcha] Erro extraindo imagens (tentativa %d): %v\n", attempt, err)
			continue
		}
		lastOuterB64, lastInnerB64 = outerB64, innerB64

		var angle float64
		angle, err = solvePuzzleWithVisionService(outerB64, innerB64)
		if err != nil {
			fmt.Printf("⚠️  [Captcha] Vision Service falhou: %v\n", err)
			angle, err = solveRotateWithSadCaptcha(outerB64, innerB64)
			if err != nil {
				fmt.Printf("⚠️  [Captcha] Ambos os métodos falharam (tentativa %d)\n", attempt)
				continue
			}
			fmt.Println("✅ [Captcha] Resolvido com SadCaptcha")
		} else {
			fmt.Println("✅ [Captcha] Resolvido com Vision Service")
		}

		if angle == 0 {
			fmt.Printf("⚠️  [Captcha] Ângulo 0 - ignorando (tentativa %d)\n", attempt)
			continue
		}

		slider, err := findSlider(page)
		if err != nil {
			fmt.Printf("⚠️  [Captcha] Slider não encontrado (tentativa %d): %v\n", attempt, err)
			continue
		}

		var l_s, l_i float64

		slidebarWidth, err := page.Eval(`() => {
			const selectors = [
				'.captcha_verify_slide--slidebar',
				'[class*="captcha_verify_slide--slidebar"]',
				'[class*="cap-w-full"][class*="cap-relative"]'
			];
			for (const sel of selectors) {
				const el = document.querySelector(sel);
				if (el) {
					const rect = el.getBoundingClientRect();
					if (rect.width > 100) return rect.width;
				}
			}
			const icon = document.querySelector('.secsdk-captcha-drag-icon');
			if (icon) {
				let parent = icon.parentElement;
				for (let i = 0; i < 5 && parent; i++) {
					const rect = parent.getBoundingClientRect();
					if (rect.width > 200) return rect.width;
					parent = parent.parentElement;
				}
			}
			return 0;
		}`)
		if err != nil || slidebarWidth.Value.Num() == 0 {
			l_s = 340.0
		} else {
			l_s = slidebarWidth.Value.Num()
		}

		iconWidth, err := page.Eval(`() => {
			const icon = document.querySelector('.secsdk-captcha-drag-icon');
			if (icon) {
				const rect = icon.getBoundingClientRect();
				return rect.width;
			}
			return 0;
		}`)
		if err != nil || iconWidth.Value.Num() == 0 {
			l_i = 64.0
		} else {
			l_i = iconWidth.Value.Num()
		}

		maxDistance := l_s - l_i
		pixelsToMove := (maxDistance * angle) / 360.0

		if pixelsToMove <= 0 {
			fmt.Printf("⚠️  [Captcha] Distância calculada inválida: %.2f (tentativa %d)\n", pixelsToMove, attempt)
			continue
		}

		fmt.Printf("🎯 [Captcha] Arrastando slider: ângulo=%.2f°, distância=%.2fpx\n", angle, pixelsToMove)

		if err := DragSlider(page, slider, pixelsToMove); err != nil {
			fmt.Printf("⚠️  [Captcha] Erro arrastando slider (tentativa %d): %v\n", attempt, err)
			continue
		}

		time.Sleep(1 * time.Second)

		if !isCaptchaPresent(page) {
			fmt.Println("🎉 [Captcha] ROTAÇÃO resolvida com sucesso!")
			return nil
		}

		fmt.Printf("⚠️  [Captcha] Ainda presente após rotação (tentativa %d)\n", attempt)
	}

	fmt.Println("⚠️  [Captcha] Tentativas automáticas esgotadas para ROTAÇÃO.")
	if lastOuterB64 != "" {
		SaveCaptchaSample("rotate", map[string]string{
			"outer": lastOuterB64,
			"inner": lastInnerB64,
		}, true)
	}
	return waitCaptchaResolution(page, 5*time.Minute)
}

// handlePuzzleCaptcha resolve captcha do tipo Puzzle usando SadCaptcha
func handlePuzzleCaptcha(page *rod.Page) error {
	fmt.Println("🧩 [Captcha] Detectado captcha de PUZZLE")

	const maxRetries = 5
	const retryDelay = 2 * time.Second

	var lastBgB64, lastPieceB64 string

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			fmt.Printf("🔄 [Captcha] Tentativa %d/%d...\n", attempt, maxRetries)
			time.Sleep(retryDelay)
		}

		bgB64, pieceB64, err := extractPuzzleImages(page)
		if err != nil {
			fmt.Printf("⚠️  [Captcha] Erro extraindo imagens (tentativa %d): %v\n", attempt, err)
			continue
		}
		lastBgB64, lastPieceB64 = bgB64, pieceB64

		var distance float64
		distance, err = solvePuzzleWithVisionService(bgB64, pieceB64)
		if err != nil {
			fmt.Printf("⚠️  [Captcha] Vision Service falhou: %v\n", err)
			distance, err = solvePuzzleWithSadCaptcha(bgB64, pieceB64)
			if err != nil {
				fmt.Printf("⚠️  [Captcha] Ambos os métodos falharam (tentativa %d)\n", attempt)
				continue
			}
			fmt.Println("✅ [Captcha] Resolvido com SadCaptcha")
		} else {
			fmt.Println("✅ [Captcha] Resolvido com Vision Service")
		}

		if distance == 0 {
			fmt.Printf("⚠️  [Captcha] Offset 0 - ignorando (tentativa %d)\n", attempt)
			continue
		}

		slider, err := findSlider(page)
		if err != nil {
			fmt.Printf("⚠️  [Captcha] Erro localizando slider (tentativa %d): %v\n", attempt, err)
			continue
		}

		fmt.Printf("🎯 [Captcha] Arrastando slider: distância=%.2fpx\n", distance)

		if err := DragSlider(page, slider, distance); err != nil {
			fmt.Printf("⚠️  [Captcha] Erro arrastando slider (tentativa %d): %v\n", attempt, err)
			continue
		}

		time.Sleep(2 * time.Second)

		if !isCaptchaPresent(page) {
			fmt.Printf("🎉 [Captcha] PUZZLE resolvido na tentativa %d!\n", attempt)
			return nil
		}

		fmt.Printf("⚠️  [Captcha] Ainda presente após tentativa %d\n", attempt)
	}

	fmt.Println("⚠️  [Captcha] Tentativas automáticas esgotadas para PUZZLE.")
	if lastBgB64 != "" {
		SaveCaptchaSample("slider", map[string]string{
			"background": lastBgB64,
			"piece":      lastPieceB64,
		}, true)
	}
	return waitCaptchaResolution(page, 5*time.Minute)
}

// extractRotateImages extrai as imagens do captcha de rotação em Base64
func extractRotateImages(page *rod.Page) (outer, inner string, err error) {
	fmt.Println("🔍 [Captcha] Extraindo imagens do captcha de rotação...")

	// Para o TikTok, há 2 imagens com alt="Captcha"
	// A primeira é o círculo externo (outer), a segunda é o círculo interno (inner)

	// Estratégia 1: Buscar por alt="Captcha"
	captchaImages, err := page.Elements("img[alt='Captcha']")
	if err == nil && len(captchaImages) >= 2 {
		fmt.Printf("✅ [Captcha] Encontradas %d imagens com alt='Captcha'\n", len(captchaImages))

		// Primeira imagem = outer (círculo externo/fundo)
		outerSrc, err := captchaImages[0].Attribute("src")
		if err == nil && outerSrc != nil {
			outer = *outerSrc
			fmt.Println("📸 [Captcha] Outer (fundo) extraído")
		}

		// Segunda imagem = inner (círculo interno/rotativo)
		innerSrc, err := captchaImages[1].Attribute("src")
		if err == nil && innerSrc != nil {
			inner = *innerSrc
			fmt.Println("📸 [Captcha] Inner (rotativo) extraído")
		}
	}

	// Estratégia 2: Buscar por classes específicas
	if outer == "" || inner == "" {
		fmt.Println("🔄 [Captcha] Tentando estratégia alternativa (por classe)...")

		// Outer: altura maior (170px ou 210px)
		if outer == "" {
			outerSelectors := []string{
				"img[class*='cap-h-[170px]']",
				"img[class*='cap-h-[210px]']",
				"img[class*='sm:cap-h-[210px]']",
			}
			for _, selector := range outerSelectors {
				if el, err := page.Timeout(1 * time.Second).Element(selector); err == nil {
					if src, err := el.Attribute("src"); err == nil && src != nil {
						outer = *src
						fmt.Printf("✅ [Captcha] Outer encontrado via: %s\n", selector)
						break
					}
				}
			}
		}

		// Inner: altura menor (105px ou 128px) + classe absolute
		if inner == "" {
			innerSelectors := []string{
				"img[class*='cap-absolute']",
				"img[class*='cap-h-[105px]']",
				"img[class*='cap-h-[128px]']",
				"img[class*='sm:cap-h-[128px]']",
			}
			for _, selector := range innerSelectors {
				if el, err := page.Timeout(1 * time.Second).Element(selector); err == nil {
					if src, err := el.Attribute("src"); err == nil && src != nil {
						inner = *src
						fmt.Printf("✅ [Captcha] Inner encontrado via: %s\n", selector)
						break
					}
				}
			}
		}
	}

	// Estratégia 3: Fallback - todas as imagens no container de captcha
	if outer == "" || inner == "" {
		fmt.Println("🔄 [Captcha] Tentando estratégia de fallback (container)...")

		containerSelectors := []string{
			"[class*='captcha-verify-container']",
			"[class*='TUXModal']",
		}

		for _, containerSel := range containerSelectors {
			container, err := page.Timeout(2 * time.Second).Element(containerSel)
			if err == nil {
				images, _ := container.Elements("img")
				if len(images) >= 2 {
					if outer == "" {
						if src, err := images[0].Attribute("src"); err == nil && src != nil {
							outer = *src
							fmt.Println("📸 [Captcha] Outer extraído do container")
						}
					}
					if inner == "" {
						if src, err := images[1].Attribute("src"); err == nil && src != nil {
							inner = *src
							fmt.Println("📸 [Captcha] Inner extraído do container")
						}
					}
					break
				}
			}
		}
	}

	// Converte as URLs/data URLs para Base64 puro
	if outer != "" {
		outer, err = downloadImageAsBase64(outer)
		if err != nil {
			return "", "", fmt.Errorf("erro processando outer: %w", err)
		}
	}

	if inner != "" {
		inner, err = downloadImageAsBase64(inner)
		if err != nil {
			return "", "", fmt.Errorf("erro processando inner: %w", err)
		}
	}

	if outer == "" || inner == "" {
		fmt.Printf("❌ [Captcha] Extração incompleta - Outer: %v, Inner: %v\n",
			outer != "", inner != "")
		return "", "", ErrCaptchaNotFound
	}

	fmt.Println("✅ [Captcha] Ambas as imagens de rotação extraídas com sucesso")
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
// Detecta automaticamente se é um captcha de rotação ou slider
func solvePuzzleWithVisionService(background, piece string) (float64, error) {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	fmt.Printf("📡 [NATS] Conectando ao servidor: %s\n", natsURL)

	// Conecta ao NATS
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return 0, fmt.Errorf("erro conectando ao NATS: %w", err)
	}
	defer nc.Close()

	// Detecta tipo de captcha (assume rotate por padrão conforme indicado pelo usuário)
	// Captcha de rotação: 2 círculos (outer e inner)
	// Captcha de slider: background e piece
	payload := map[string]string{
		"outer_b64": background,
		"inner_b64": piece,
	}

	payloadBytes, _ := json.Marshal(payload)

	fmt.Println("📤 [NATS] Enviando requisição para jobs.captcha.slider (tipo: ROTATE)...")

	// Envia requisição e aguarda resposta (request-reply pattern)
	msg, err := nc.Request("jobs.captcha.slider", payloadBytes, 30*time.Second)
	if err != nil {
		return 0, fmt.Errorf("erro na requisição NATS: %w", err)
	}

	// DEBUG: Log da resposta bruta
	fmt.Printf("🐛 [NATS] Resposta bruta: %s\n", string(msg.Data))

	// Parse resposta (pode ser angle para rotate ou x_offset para slider)
	var response struct {
		// Campos para rotate
		Angle float64 `json:"angle"`
		// Campos para slider
		XOffset float64 `json:"x_offset"`
		// Campos comuns
		Success    bool    `json:"success"`
		Confidence float64 `json:"confidence"`
		Error      string  `json:"error"`
	}

	if err := json.Unmarshal(msg.Data, &response); err != nil {
		return 0, fmt.Errorf("erro parseando resposta: %w", err)
	}

	// DEBUG: Log do struct parseado
	fmt.Printf("🐛 [NATS] Struct: success=%v, angle=%.2f, x_offset=%.2f, confidence=%.4f, error='%s'\n",
		response.Success, response.Angle, response.XOffset, response.Confidence, response.Error)

	if !response.Success {
		return 0, fmt.Errorf("Vision Service falhou: %s", response.Error)
	}

	// Retorna angle para rotate, x_offset para slider
	result := response.Angle
	if result == 0 && response.XOffset != 0 {
		result = response.XOffset
	}

	fmt.Printf("✅ [NATS] Resposta recebida: resultado = %.2f (confiança: %.2f%%)\n",
		result, response.Confidence*100)

	// Avisa se a confiança é muito baixa
	if response.Confidence < 0.3 {
		fmt.Printf("⚠️  [NATS] Confiança baixa (%.1f%%). Resultado pode não ser preciso.\n",
			response.Confidence*100)
	}

	return result, nil
}

// extractCaptchaImages detecta e extrai as URLs das imagens do captcha
// Retorna a URL da imagem de fundo e da peça do quebra-cabeça
func extractCaptchaImages(page *rod.Page) (*CaptchaImages, error) {
	fmt.Println("🔍 [Captcha] Procurando elementos de imagem...")

	// DEBUG: Lista TODOS os elementos visíveis na página
	fmt.Println("🐛 [Debug] Listando estrutura do DOM...")
	debugElements(page)

	// Estratégia 1: Procurar por iframe do captcha
	iframe, err := page.Timeout(3 * time.Second).Element(`iframe[src*="captcha"]`)
	if err == nil {
		fmt.Println("✅ [Captcha] Iframe de captcha encontrado")
		// Se encontrou iframe, entra nele
		page = iframe.MustFrame()
		fmt.Println("🐛 [Debug] Listando estrutura do iframe...")
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
				fmt.Printf("✅ [Captcha] Background encontrado via: %s\n", selector)
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
				fmt.Printf("✅ [Captcha] Piece encontrado via: %s\n", selector)
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
					fmt.Println("✅ [Captcha] Background extraído de canvas")
				} else if i == 1 && pieceURL == "" {
					pieceURL = url
					fmt.Println("✅ [Captcha] Piece extraído de canvas")
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
				fmt.Printf("📸 [Captcha] Encontradas %d imagens no container '%s'\n", len(images), containerSel)

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

	fmt.Println("✅ [Captcha] Ambas as imagens extraídas com sucesso")
	return &CaptchaImages{
		BackgroundURL: backgroundURL,
		PieceURL:      pieceURL,
	}, nil
}

// findSlider localiza o elemento do slider que deve ser arrastado
// Usa o seletor padrão .secsdk-captcha-drag-icon conforme documentação SadCaptcha
func findSlider(page *rod.Page) (*rod.Element, error) {
	// Seletores ordenados por prioridade (documentação primeiro)
	sliderSelectors := []string{
		// Seletor primário (documentação SadCaptcha)
		".secsdk-captcha-drag-icon",
		`[class*="secsdk-captcha-drag-icon"]`,
		`button[class*="secsdk-captcha-drag-icon"]`,

		// Seletores alternativos TikTok
		`[class*="captcha-drag-icon"]`,
		`div[class*="secsdk-captcha-drag"]`,
		`[class*="captcha_verify"] [class*="drag"]`,
		`div[class*="slider"][class*="button"]`,
		`[class*="verify-bar"] [class*="verify-slide"]`,

		// Seletores para nova UI TikTok (cap- prefix)
		`[class*="cap-absolute"][class*="cap-cursor"]`,
		`div[class*="cap-w-6"][class*="cap-h-6"]`,
		`svg[class*="cap-absolute"]`,

		// Fallback genérico
		`button[class*="TUXButton"][class*="drag"]`,
		`.captcha-verify-container button`,
		`[class*="captcha-verify"] button`,
	}

	fmt.Println("🔍 [Captcha] Procurando elemento slider (.secsdk-captcha-drag-icon)...")

	for _, selector := range sliderSelectors {
		elements, err := page.Timeout(500 * time.Millisecond).Elements(selector)
		if err != nil || len(elements) == 0 {
			continue
		}

		// Testa cada elemento encontrado
		for _, el := range elements {
			// Verifica se o elemento é visível
			visible, _ := el.Visible()
			if !visible {
				continue
			}

			// Verifica dimensões
			box, err := el.Shape()
			if err != nil || len(box.Quads) == 0 {
				continue
			}

			quad := box.Quads[0]
			width := quad[2] - quad[0]
			height := quad[5] - quad[1]

			// Slider icon geralmente tem 20-50px
			if width >= 15 && height >= 15 && width <= 80 && height <= 80 {
				fmt.Printf("✅ [Captcha] Slider encontrado via: %s (%.0fx%.0f)\n", selector, width, height)
				return el, nil
			} else if width > 10 && height > 10 {
				fmt.Printf("⚠️  [Captcha] Elemento com tamanho atípico via %s: %.0fx%.0f (tentando)\n", selector, width, height)
				return el, nil
			}
		}
	}

	// DEBUG: Lista todos os elementos com 'captcha' ou 'slide' na classe
	fmt.Println("🐛 [Debug] Listando elementos relacionados a captcha...")
	debugSelectors := []string{"[class*='captcha']", "[class*='slide']", "[class*='secsdk']"}
	for _, sel := range debugSelectors {
		elements, _ := page.Elements(sel)
		if len(elements) > 0 {
			fmt.Printf("  Encontrados %d elementos com '%s'\n", len(elements), sel)
		}
	}

	return nil, fmt.Errorf("slider não encontrado na página (testados %d seletores)", len(sliderSelectors))
}

// waitCaptchaResolution aguarda até que o CAPTCHA seja resolvido manualmente
// ou até que o tempo limite seja atingido.
// O browser permanece aberto — resolva o captcha e a automação continuará automaticamente.
func waitCaptchaResolution(page *rod.Page, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)

	fmt.Println("════════════════════════════════════════════")
	fmt.Println("🛑  CAPTCHA DETECTADO — INTERVENÇÃO MANUAL  ")
	fmt.Println("════════════════════════════════════════════")
	fmt.Printf("⏳ Você tem %s para resolver o captcha.\n", maxWait)
	fmt.Println("   Resolva o captcha no browser aberto e aguarde.")
	fmt.Println("════════════════════════════════════════════")

	for time.Now().Before(deadline) {
		if !isCaptchaPresent(page) {
			fmt.Println("✅ [Captcha] Resolvido manualmente! Continuando automação...")
			return nil
		}
		remaining := time.Until(deadline).Round(time.Second)
		fmt.Printf("⏳ [Captcha Manual] Aguardando resolução... (%s restantes)\n", remaining)
		time.Sleep(2 * time.Second)
	}

	fmt.Println("❌ [Captcha] Tempo limite esgotado para resolução manual.")
	return ErrCaptchaTimeout
}
