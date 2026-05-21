

package tiktok

import (
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/loviiin/project-argus/pkg/captcha"
)

// handleCaptcha lida com a aparição do CAPTCHA.
// Agora usa exclusivamente a Tática do Fantasma Ignorante: tenta fechar o overlay via pkg/captcha.
func (s *Source) handleCaptcha(page *rod.Page, ctxStr string) error {
	if dismissed, err := captcha.DismissOverlay(page, ctxStr); err == nil && dismissed {
		fmt.Printf("[%s] 👻 [Ghost] CAPTCHA dismissed! Continuando coleta...\n", ctxStr)
		
		if err := captcha.WaitForContent(page, `a[href*="/video/"]`); err != nil {
			fmt.Printf("[%s] ⚠️ [Ghost] Conteúdo não carregou pós-dismiss: %v\n", ctxStr, err)
			return ErrCaptchaTimeout
		}
		return nil
	}

	return ErrCaptcha
}

// detectCaptchaType identifica qual tipo de captcha está presente na página
func detectCaptchaType(page *rod.Page) CaptchaType {
	if _, err := page.Timeout(500*time.Millisecond).ElementR("*", "(?i)(fit.*puzzle|encaixe.*peça)"); err == nil {
		return CaptchaTypePuzzle
	}
	return CaptchaTypeSoftGate
}
