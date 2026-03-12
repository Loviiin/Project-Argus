package captcha

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/google/uuid"
)

// RotationLabel é o JSON final salvo para cada sample de treinamento.
type RotationLabel struct {
	ID            string  `json:"id"`
	Angle         float64 `json:"angle"`
	RawPixels     float64 `json:"raw_pixels"`
	SlidebarWidth float64 `json:"slidebar_width"`
	IconWidth     float64 `json:"icon_width"`
	Timestamp     string  `json:"timestamp"`
}

var ErrCaptchaTimeout = errors.New("timeout aguardando resolução do captcha")
var ErrCaptchaNotFound = errors.New("elementos do captcha não encontrados")

// RunShadowCollector executa o fluxo completo do Shadow Collector para extração
func RunShadowCollector(page *rod.Page, datasetPath string, origin string) error {
	ctxStr := origin
	if err := os.MkdirAll(datasetPath, 0755); err != nil {
		return fmt.Errorf("erro criando diretório dataset '%s': %w", datasetPath, err)
	}

	// BLINDAGEM CONTRA I/O RACE CONDITIONS (UUID)
	id := fmt.Sprintf("%d_%s", time.Now().UnixMilli(), uuid.New().String()[:8])
	fmt.Printf("[%s] 🕵️  [Shadow] Iniciando coleta — ID: %s\n", ctxStr, id)

	// ─── Passo 1: Extrair e salvar imagens ──────────────────────────
	outerB64, innerB64, err := ExtractRotateImages(page)
	if err != nil {
		return fmt.Errorf("erro extraindo imagens: %w", err)
	}

	outerPath := filepath.Join(datasetPath, id+"_outer.jpg")
	innerPath := filepath.Join(datasetPath, id+"_inner.jpg")

	if err := saveBase64Image(outerB64, outerPath); err != nil {
		return fmt.Errorf("erro salvando outer: %w", err)
	}
	if err := saveBase64Image(innerB64, innerPath); err != nil {
		cleanupShadowFiles(id, datasetPath)
		return fmt.Errorf("erro salvando inner: %w", err)
	}

	fmt.Printf("[%s] 📸 [Shadow] Imagens salvas: %s, %s\n", ctxStr, outerPath, innerPath)

	// ─── Passo 2: Shadow Polling — aguardar VNC + capturar slider ───
	fmt.Printf("[%s] ════════════════════════════════════════════\n", ctxStr)
	fmt.Printf("[%s] 🕵️  SHADOW COLLECTOR — AGUARDANDO VNC       \n", ctxStr)
	fmt.Printf("[%s] ════════════════════════════════════════════\n", ctxStr)
	fmt.Printf("[%s]    Resolva o captcha manualmente no browser.\n", ctxStr)
	fmt.Printf("[%s]    O coletor NÃO interfere no mouse.\n", ctxStr)
	fmt.Printf("[%s] ════════════════════════════════════════════\n", ctxStr)

	captchaPage := page
	if iframe, err := page.Timeout(3 * time.Second).Element(`iframe[src*="captcha"]`); err == nil {
		if frame := iframe.MustFrame(); frame != nil {
			captchaPage = frame
			fmt.Printf("[%s] 🔍 [Shadow] Captcha detectado dentro de iframe\n", ctxStr)
		}
	}

	const maxWait = 5 * time.Minute
	// Polling agressivo: 50ms para minimizar lag do VNC entre a posição
	// real do slider e o snapshot capturado.
	const pollInterval = 50 * time.Millisecond
	const dWindowSize = 5

	// Detecção de estabilidade: quando D para de mudar por stabilityRequired
	// polls consecutivos, o usuário soltou o slider. Congelamos D nesse
	// instante — antes do captcha desaparecer do DOM.
	const stabilityThresholdPx = 1.5 // delta máximo (px) para considerar "parado"
	// 8 polls × 50ms = ~400ms de estabilidade para confirmar release.
	// 4 era pouco: uma pausa momentânea durante o arrasto pode durar 200ms
	// e acionar o freeze no lugar errado. Pausas reais raramente passam de 200ms.
	const stabilityRequired = 8

	deadline := time.Now().Add(maxWait)

	var dWindow []float64  // fallback: últimos dWindowSize valores válidos de D
	var frozenD float64    // posição congelada no momento do release detectado
	var prevD float64 = -1 // último D lido para calcular delta (sentinela: -1)
	var stableCount int    // contador de polls consecutivos com D estável
	var lastLs, lastLi float64
	var userStartedDragging bool
	logTicker := time.Now()

	const sliderJS = `() => {
		const icon = document.querySelector('.secsdk-captcha-drag-icon') || document.querySelector('#captcha_slide_button');
		const bar = document.querySelector('.captcha_verify_slide--slidebar') || 
					document.querySelector('.cap-bg-UISheetGrouped3') || 
					document.querySelector('[class*="cap-bg-UISheetGrouped3"]');
					
		if (!icon || !bar) return null;

		let d = 0;
		if (icon.parentElement && icon.parentElement.style && icon.parentElement.style.transform) {
			const transform = icon.parentElement.style.transform;
			const m = transform.match(/translateX\(([-.\d]+)px\)/);
			if (m) d = parseFloat(m[1]);
		}
		if (d <= 0) {
			const transform = getComputedStyle(icon).transform;
			if (transform && transform !== 'none') {
				const m = transform.match(/matrix\([^,]*,\s*[^,]*,\s*[^,]*,\s*[^,]*,\s*([\d.]+)/);
				if (m) d = parseFloat(m[1]);
			}
		}
		if (d <= 0) {
			let left = parseFloat(getComputedStyle(icon).left);
			if (left > 0) d = left;
			else if (icon.parentElement) {
				left = parseFloat(getComputedStyle(icon.parentElement).left);
				if (left > 0) d = left;
			}
		}
		if (d <= 0) {
			d = icon.getBoundingClientRect().left - bar.getBoundingClientRect().left;
		}

		return JSON.stringify({
			d: d,
			ls: bar.getBoundingClientRect().width,
			li: icon.getBoundingClientRect().width || icon.clientWidth
		});
	}`

	resolved := false
	for time.Now().Before(deadline) {
		if result, err := captchaPage.Eval(sliderJS); err == nil && result.Value.Str() != "" {
			var snap struct {
				D  float64 `json:"d"`
				Ls float64 `json:"ls"`
				Li float64 `json:"li"`
			}
			if json.Unmarshal([]byte(result.Value.Str()), &snap) == nil && snap.D > 0 {
				if snap.Ls > 0 {
					lastLs = snap.Ls
				}
				if snap.Li > 0 {
					lastLi = snap.Li
				}

				if lastLi > 0 && snap.D > lastLi*0.3 {
					userStartedDragging = true
				}

				if userStartedDragging {
					// Fallback window — mantém os últimos N leituras
					dWindow = append(dWindow, snap.D)
					if len(dWindow) > dWindowSize {
						dWindow = dWindow[len(dWindow)-dWindowSize:]
					}

					// Se já temos frozenD mas D mudou significativamente,
					// usuário voltou a arrastar — reseta o freeze
					if frozenD > 0 {
						diff := snap.D - frozenD
						if diff < 0 {
							diff = -diff
						}
						if diff > stabilityThresholdPx*3 {
							fmt.Printf("🔓 [Shadow] Freeze resetado — usuário voltou a arrastar "+
								"(D=%.2f vs frozenD=%.2f)\n", snap.D, frozenD)
							frozenD = 0
							stableCount = 0
							prevD = snap.D
						}
					}

					// Detecção de release por estabilidade:
					// enquanto o usuário arrasta, D muda a cada poll.
					// Quando solta, D fica constante — detectamos isso aqui
					// e congelamos antes do DOM desaparecer.
					if frozenD == 0 {
						delta := snap.D - prevD
						if delta < 0 {
							delta = -delta
						}
						// snap.D > lastLi*0.15 garante que só contamos estabilidade quando
						// o slider já avançou pelo menos 15% da largura do ícone.
						// Isso evita congelar D durante a pausa antes de começar a arrastar.
						if prevD >= 0 && snap.D > (lastLi*0.15) && delta <= stabilityThresholdPx {
							stableCount++
							if stableCount >= stabilityRequired {
								frozenD = snap.D
								fmt.Printf("🔒 [Shadow] Release detectado por estabilidade: D=%.2f (após %d polls estáveis)\n",
									frozenD, stableCount)
							}
						} else {
							stableCount = 0 // reset: slider em movimento ou ainda no início
						}
						prevD = snap.D
					}
				}
			}
		}

		if !IsCaptchaPresent(captchaPage) {
			// Confirmado empiricamente: o slider desaparece instantaneamente com o DOM.
			// Polls extras retornam snap.D = 0 e poluem a dWindow, corrompendo o fallback.
			// Removido: qualquer tentativa de coletar dados após DOM sumir.
			resolved = true
			break
		}

		if time.Since(logTicker) >= 5*time.Second {
			remaining := time.Until(deadline).Round(time.Second)
			var latestD float64
			if len(dWindow) > 0 {
				latestD = dWindow[len(dWindow)-1]
			}
			if frozenD > 0 {
				fmt.Printf("⏳ [Shadow] Aguardando DOM... (%s restantes, frozenD=%.1f 🔒)\n", remaining, frozenD)
			} else if !userStartedDragging {
				fmt.Printf("⏳ [Shadow] Aguardando início do arrasto... (%s restantes, D=%.1f)\n", remaining, latestD)
			} else {
				fmt.Printf("⏳ [Shadow] Aguardando resolução... (%s restantes, D=%.1f, estável=%d/%d)\n",
					remaining, latestD, stableCount, stabilityRequired)
			}
			logTicker = time.Now()
		}

		time.Sleep(pollInterval)
	}

	if !resolved {
		fmt.Println("❌ [Shadow] Timeout — limpando arquivos...")
		cleanupShadowFiles(id, datasetPath)
		return ErrCaptchaTimeout
	}

	fmt.Println("✅ [Shadow] Captcha resolvido!")

	// Prioridade 1: posição congelada no momento em que D estabilizou
	// (slider solto) — capturada antes do DOM desaparecer.
	// Prioridade 2: mediana da janela deslizante como fallback se o
	// captcha desapareceu antes de detectarmos estabilidade (raro).
	var d float64
	var dSource string
	if frozenD > 0 {
		d = frozenD
		dSource = "frozen@release"
	} else {
		d = medianFloat64(dWindow)
		fmt.Printf("⚠️  [Shadow] FALLBACK: frozenD não detectado antes do DOM sumir. "+
			"Usando mediana de %d amostras (d=%.2f). "+
			"Considere aumentar stabilityRequired se isso ocorrer frequentemente.\n",
			len(dWindow), d)
		dSource = fmt.Sprintf("median(%d amostras)", len(dWindow))
	}
	ls := lastLs
	li := lastLi

	fmt.Printf("📐 [Shadow] Metadados (%s): d=%.2f, l_s=%.2f, l_i=%.2f\n", dSource, d, ls, li)

	if d <= 0 || ls <= 0 || li <= 0 || ls <= li {
		fmt.Printf("❌ [Shadow] Metadados inválidos (d=%.2f, ls=%.2f, li=%.2f) — descartando.\n", d, ls, li)
		cleanupShadowFiles(id, datasetPath)
		return fmt.Errorf("metadados do slider inválidos")
	}

	angle := (d * 360.0) / (ls - li)

	if angle < 0 || angle > 360 {
		fmt.Printf("❌ [Shadow] Ângulo fora do range (%.2f°) — descartando.\n", angle)
		cleanupShadowFiles(id, datasetPath)
		return fmt.Errorf("ângulo calculado fora do range: %.2f", angle)
	}

	fmt.Printf("🎯 [Shadow] Ângulo calculado: %.2f°\n", angle)

	label := RotationLabel{
		ID:            id,
		Angle:         angle,
		RawPixels:     d,
		SlidebarWidth: ls,
		IconWidth:     li,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}

	labelPath := filepath.Join(datasetPath, id+"_label.json")
	labelData, _ := json.MarshalIndent(label, "", "  ")
	if err := os.WriteFile(labelPath, labelData, 0644); err != nil {
		fmt.Printf("❌ [Shadow] Erro salvando label: %v\n", err)
		cleanupShadowFiles(id, datasetPath)
		return fmt.Errorf("erro salvando label: %w", err)
	}

	fmt.Println("════════════════════════════════════════════")
	fmt.Printf("✅ [Shadow] Sample coletado com sucesso!\n")
	fmt.Printf("   ID:     %s\n", id)
	fmt.Printf("   Ângulo: %.2f°\n", angle)
	fmt.Printf("   Pixels: %.2f\n", d)
	fmt.Printf("   Label:  %s\n", labelPath)
	fmt.Println("════════════════════════════════════════════")

	return nil
}

// extractRotateImageURLs localiza as URLs das imagens outer e inner no DOM.
// Função interna reutilizada por ExtractRotateImages e ExtractRotateImageBytes.
func extractRotateImageURLs(page *rod.Page) (outerURL, innerURL string, err error) {
	fmt.Println("🔍 [Captcha] Extraindo URLs das imagens do captcha de rotação...")

	// Estratégia 1: Buscar por alt="Captcha"
	captchaImages, err := page.Elements("img[alt='Captcha']")
	if err == nil && len(captchaImages) >= 2 {
		outerSrc, _ := captchaImages[0].Attribute("src")
		if outerSrc != nil {
			outerURL = *outerSrc
		}

		innerSrc, _ := captchaImages[1].Attribute("src")
		if innerSrc != nil {
			innerURL = *innerSrc
		}
	}

	// Estratégia 2: Extensões por CSS
	if outerURL == "" || innerURL == "" {
		if outerURL == "" {
			outerSelectors := []string{
				"img[class*='cap-h-[170px]']",
				"img[class*='cap-h-[210px]']",
				"img[class*='sm:cap-h-[210px]']",
			}
			for _, selector := range outerSelectors {
				if el, err := page.Timeout(1 * time.Second).Element(selector); err == nil {
					if src, err := el.Attribute("src"); err == nil && src != nil {
						outerURL = *src
						break
					}
				}
			}
		}
		if innerURL == "" {
			innerSelectors := []string{
				"img[class*='cap-absolute']",
				"img[class*='cap-h-[105px]']",
				"img[class*='cap-h-[128px]']",
				"img[class*='sm:cap-h-[128px]']",
			}
			for _, selector := range innerSelectors {
				if el, err := page.Timeout(1 * time.Second).Element(selector); err == nil {
					if src, err := el.Attribute("src"); err == nil && src != nil {
						innerURL = *src
						break
					}
				}
			}
		}
	}

	// Estratégia 3: Fallback container
	if outerURL == "" || innerURL == "" {
		containerSelectors := []string{
			"[class*='captcha-verify-container']",
			"[class*='TUXModal']",
		}

		for _, containerSel := range containerSelectors {
			container, err := page.Timeout(2 * time.Second).Element(containerSel)
			if err == nil {
				images, _ := container.Elements("img")
				if len(images) >= 2 {
					if outerURL == "" {
						if src, err := images[0].Attribute("src"); err == nil && src != nil {
							outerURL = *src
						}
					}
					if innerURL == "" {
						if src, err := images[1].Attribute("src"); err == nil && src != nil {
							innerURL = *src
						}
					}
					break
				}
			}
		}
	}

	if outerURL == "" || innerURL == "" {
		return "", "", ErrCaptchaNotFound
	}

	return outerURL, innerURL, nil
}

// ExtractRotateImageBytes extrai as imagens do captcha de rotação como bytes crus.
// Retorna os binários das imagens (PNG/JPEG/WebP) prontos para inferência ONNX.
func ExtractRotateImageBytes(page *rod.Page) (outer, inner []byte, err error) {
	outerURL, innerURL, err := extractRotateImageURLs(page)
	if err != nil {
		return nil, nil, err
	}

	outer, err = DownloadImageRaw(outerURL)
	if err != nil {
		return nil, nil, fmt.Errorf("erro baixando outer: %w", err)
	}

	inner, err = DownloadImageRaw(innerURL)
	if err != nil {
		return nil, nil, fmt.Errorf("erro baixando inner: %w", err)
	}

	return outer, inner, nil
}

// ExtractRotateImages extrai as imagens do captcha de rotação em Base64.
// Mantido para compatibilidade com fallbacks (NATS, SadCaptcha).
func ExtractRotateImages(page *rod.Page) (outer, inner string, err error) {
	outerURL, innerURL, err := extractRotateImageURLs(page)
	if err != nil {
		return "", "", err
	}

	outer, err = DownloadImageAsBase64(outerURL)
	if err != nil {
		return "", "", fmt.Errorf("erro processando outer: %w", err)
	}

	inner, err = DownloadImageAsBase64(innerURL)
	if err != nil {
		return "", "", fmt.Errorf("erro processando inner: %w", err)
	}

	return outer, inner, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func DownloadImageAsBase64(imageURL string) (string, error) {
	if strings.HasPrefix(imageURL, "data:image") {
		parts := strings.Split(imageURL, ",")
		if len(parts) > 1 {
			return parts[1], nil
		}
	}

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

// DownloadImageRaw baixa uma imagem por URL e retorna os bytes crus.
// Trata data URIs extraindo e decodificando o base64 embutido.
func DownloadImageRaw(imageURL string) ([]byte, error) {
	if strings.HasPrefix(imageURL, "data:image") {
		parts := strings.Split(imageURL, ",")
		if len(parts) > 1 {
			return base64.StdEncoding.DecodeString(parts[1])
		}
	}

	resp, err := http.Get(imageURL)
	if err != nil {
		return nil, fmt.Errorf("erro baixando imagem: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d ao baixar imagem", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func saveBase64Image(b64, path string) error {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("erro decodificando base64: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// medianFloat64 retorna a mediana de uma slice de float64.
// Retorna 0 se a slice estiver vazia.
func medianFloat64(vals []float64) float64 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	tmp := make([]float64, n)
	copy(tmp, vals)
	sort.Float64s(tmp)
	if n%2 == 1 {
		return tmp[n/2]
	}
	return (tmp[n/2-1] + tmp[n/2]) / 2.0
}

func cleanupShadowFiles(id, dir string) {
	files := []string{
		filepath.Join(dir, id+"_outer.jpg"),
		filepath.Join(dir, id+"_inner.jpg"),
		filepath.Join(dir, id+"_label.json"),
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			fmt.Printf("⚠️  [Shadow] Erro removendo %s: %v\n", f, err)
		}
	}
}
