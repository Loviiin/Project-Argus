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
	const pollInterval = 500 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	var lastD, lastLs, lastLi float64
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
				lastD, lastLs, lastLi = snap.D, snap.Ls, snap.Li
			}
		}

		if !isCaptchaPresent(page) {
			resolved = true
			break
		}

		if time.Since(logTicker) >= 5*time.Second {
			remaining := time.Until(deadline).Round(time.Second)
			fmt.Printf("⏳ [Shadow] Aguardando resolução... (%s restantes, lastD=%.1f)\n", remaining, lastD)
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

	d := lastD
	ls := lastLs
	li := lastLi

	fmt.Printf("📐 [Shadow] Metadados (último snapshot): d=%.2f, l_s=%.2f, l_i=%.2f\n", d, ls, li)

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

func isCaptchaPresent(page *rod.Page) bool {
	info, _ := page.Info()
	urlStr := ""
	if info != nil {
		urlStr = info.URL
	}

	if strings.Contains(strings.ToLower(urlStr), "verify") ||
		strings.Contains(strings.ToLower(urlStr), "captcha") {
		return true
	}

	if _, err := page.Timeout(2 * time.Second).Element(`iframe[src*="captcha"]`); err == nil {
		return true
	}

	for _, sel := range []string{
		".captcha_verify_container",
		".captcha_verify_img_slide",
		"[class*='captcha']",
		"[class*='secsdk-captcha']",
		"[id*='captcha']",
		"div[class*='verify']",
	} {
		if _, err := page.Timeout(1 * time.Second).Element(sel); err == nil {
			return true
		}
	}

	if _, err := page.Timeout(1*time.Second).ElementR("*", "(?i)(drag.*slider|fit.*puzzle|verify|captcha)"); err == nil {
		return true
	}

	return false
}

// ExtractRotateImages extrai as imagens do captcha de rotação em Base64
func ExtractRotateImages(page *rod.Page) (outer, inner string, err error) {
	fmt.Println("🔍 [Captcha] Extraindo imagens do captcha de rotação...")

	// Estratégia 1: Buscar por alt="Captcha"
	captchaImages, err := page.Elements("img[alt='Captcha']")
	if err == nil && len(captchaImages) >= 2 {
		outerSrc, _ := captchaImages[0].Attribute("src")
		if outerSrc != nil {
			outer = *outerSrc
		}

		innerSrc, _ := captchaImages[1].Attribute("src")
		if innerSrc != nil {
			inner = *innerSrc
		}
	}

	// Estratégia 2: Extensões por CSS
	if outer == "" || inner == "" {
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
						break
					}
				}
			}
		}
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
						break
					}
				}
			}
		}
	}

	// Estratégia 3: Fallback container
	if outer == "" || inner == "" {
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
						}
					}
					if inner == "" {
						if src, err := images[1].Attribute("src"); err == nil && src != nil {
							inner = *src
						}
					}
					break
				}
			}
		}
	}

	if outer != "" {
		outer, err = DownloadImageAsBase64(outer)
		if err != nil {
			return "", "", fmt.Errorf("erro processando outer: %w", err)
		}
	}

	if inner != "" {
		inner, err = DownloadImageAsBase64(inner)
		if err != nil {
			return "", "", fmt.Errorf("erro processando inner: %w", err)
		}
	}

	if outer == "" || inner == "" {
		return "", "", ErrCaptchaNotFound
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

func saveBase64Image(b64, path string) error {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("erro decodificando base64: %w", err)
	}
	return os.WriteFile(path, data, 0644)
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
