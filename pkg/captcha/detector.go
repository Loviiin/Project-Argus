package captcha

import (
	"strings"
	"time"

	"github.com/go-rod/rod"
)

// IsCaptchaPresent verifica se um captcha está presente na página.
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

	// Tenta checar rápido se existe IFrame do Captcha (comum no TikTok)
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

	// Regex textual rápido em vez de Eval custoso
	if _, err := page.Timeout(1 * time.Second).ElementR("*", "(?i)(drag.*slider|fit.*puzzle|verify|captcha)"); err == nil {
		return true
	}

	return false
}
