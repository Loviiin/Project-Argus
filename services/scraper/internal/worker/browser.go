package worker

import (
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/loviiin/project-argus/pkg/config"
)

// NewBrowser cria uma instância de browser Rod com estado persistente.
// O UserDataDir garante que cookies, localStorage e tokens de segurança
// sejam mantidos entre execuções, evitando captchas repetidos.
func NewBrowser(stateDir string, debugPort string) (*rod.Browser, error) {
	path, _ := launcher.LookPath()

	l := launcher.New().
		Bin(path).
		UserDataDir(stateDir).
		Leakless(false).
		Devtools(true).
		Set("autoplay-policy", "no-user-gesture-required"). // Permite autoplay de vídeos
		Set("use-gl", "swiftshader").                       // Software rendering para containers
		Set("disable-gpu").                                 // Evita problemas de GPU em containers
		Set("no-sandbox")                                   // Necessário em containers Linux

	cfg := config.LoadConfig()
	if cfg.Browser.Headless {
		l = l.Set("headless", "new") // Para produção (Evasão Anti-Bot)
	} else {
		l = l.Headless(false) // Para desenvolvimento/VNC (Permite ver a tela)
	}

	u, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("erro ao iniciar browser: %w", err)
	}

	browser := rod.New().ControlURL(u).MustConnect()

	// Monitor para debug remoto
	go browser.ServeMonitor(debugPort)

	// O Scraper Worker precisa iniciar com uma página Stealth já configurada
	// para que as abas subsequentes herdem isso ou a primeira aba já navegue mascarada.
	// O stealth é aplicado por página.
	return browser, nil
}
