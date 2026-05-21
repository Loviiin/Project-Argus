package captcha

import (
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
)

// DismissOverlay tenta fechar o overlay do CAPTCHA sem resolvê-lo.
func DismissOverlay(page *rod.Page, ctxStr string) (bool, error) {
	fmt.Printf("[%s] 👻 [Ghost] Tentando fechar overlay de CAPTCHA...\n", ctxStr)

	closeSelectors := []string{
		`svg path[d^="M10.19 36.19"]`, // O botão exato de fechar do TikTok (Prioridade Máxima)
		`.captcha-verify-close`,
		`[class*="captcha"] [class*="close"]`,
		`[class*="captcha"] button[aria-label="Close"]`,
		`[class*="captcha"] svg`,
		`[class*="TUXModal"] [class*="close"]`,
		`[class*="TUXModal"] button[aria-label="Close"]`,
		`div[class*="DivCloseIcon"]`,
		`[class*="verify"] svg[class*="close"]`,
		`button[class*="close"]`,
		`[role="dialog"] button[aria-label="Close"]`,
		`[role="dialog"] [class*="close"]`,
	}

	dismissed := false
	for _, selector := range closeSelectors {
		// Busca todos os elementos que dão match no seletor (pode haver mais de um botão de fechar)
		elements, err := page.Elements(selector)
		if err == nil && len(elements) > 0 {
			for _, el := range elements {
				if visible, _ := el.Visible(); visible {
					fmt.Printf("[%s] 👻 [Ghost] Clicando no botão de fechar via: %s...\n", ctxStr, selector)
					el.Click("left", 1)
					dismissed = true
					time.Sleep(500 * time.Millisecond) // Espera um pouco para animação
				}
			}
		}
		
		// Se já não houver mais captcha na página, podemos parar a busca
		if !IsCaptchaPresent(page) {
			return true, nil
		}
	}

	if !dismissed || IsCaptchaPresent(page) {
		fmt.Printf("[%s] 👻 [Ghost] CAPTCHA ainda presente. Tentando tecla ESC...\n", ctxStr)
		page.Keyboard.Press(input.Escape)
		time.Sleep(1 * time.Second)
	}

	time.Sleep(2 * time.Second)
	if !IsCaptchaPresent(page) {
		return true, nil
	}

	return false, fmt.Errorf("overlay ainda presente após tentativas de dismiss")
}

// WaitForContent aguarda o carregamento de conteúdo real após fechar o captcha
func WaitForContent(page *rod.Page, selector string) error {
	page.Mouse.Scroll(0, 500, 1)
	time.Sleep(1 * time.Second)

	_, err := page.Timeout(10 * time.Second).Element(selector)
	return err
}
