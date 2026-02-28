package captcha

import (
	"strings"

	"github.com/go-rod/rod"
)

// IsCaptchaPresent verifica se um captcha está presente na página de forma não-bloqueante.
func IsCaptchaPresent(page *rod.Page) bool {
	info, _ := page.Info()
	urlStr := ""
	if info != nil {
		urlStr = info.URL
	}

	if strings.Contains(strings.ToLower(urlStr), "verify") ||
		strings.Contains(strings.ToLower(urlStr), "captcha") {
		return true
	}

	// Tenta checar rápido se existe IFrame do Captcha (comum no TikTok) - Não bloqueante
	if els, err := page.Elements(`iframe[src*="captcha"]`); err == nil && len(els) > 0 {
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
		if els, err := page.Elements(sel); err == nil && len(els) > 0 {
			return true
		}
	}

	// Regex textual rápido usando Eval (instantâneo) em vez de percorrer toda a árvore com ElementR
	res, err := page.Eval(`() => document.body && /drag.*slider|fit.*puzzle|verify|captcha/i.test(document.body.innerText)`)
	if err == nil && res.Value.Bool() {
		return true
	}

	return false
}
