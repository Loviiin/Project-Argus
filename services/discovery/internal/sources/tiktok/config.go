package tiktok

import (
	"fmt"

	"github.com/loviiin/project-argus/pkg/config"
)

// MovementConfig controla features de movimento (Open Core)
type MovementConfig struct {
	Enabled     bool
	Bezier      bool
	Overshoot   bool
	MicroPauses bool
	Tremor      bool
}

// DelayConfig controla timing (Open Core)
type DelayConfig struct {
	Type     string // "gaussian" (premium) ou "fixed" (open)
	MinMS    int
	MaxMS    int
	StepsMin int
	StepsMax int
}

// AntiDetectionConfig controla features avançadas (Closed Source)
type AntiDetectionConfig struct {
	TLSFingerprint    bool
	CookieRotation    bool
	UserAgentRotation bool
	ViewportRandomize bool
}

// CaptchaConfig agrega todas as configs
type CaptchaConfig struct {
	Movement      MovementConfig
	Delays        DelayConfig
	AntiDetection AntiDetectionConfig
}

// LoadCaptchaConfig carrega configurações do config.yaml
func LoadCaptchaConfig() CaptchaConfig {
	cfg := config.LoadConfig()

	// Valores padrão (Open Source básico)
	captchaCfg := CaptchaConfig{
		Movement: MovementConfig{
			Enabled:     false, // Desabilitado por padrão (open source)
			Bezier:      false,
			Overshoot:   false,
			MicroPauses: false,
			Tremor:      false,
		},
		Delays: DelayConfig{
			Type:     "fixed", // Open source = delays fixos
			MinMS:    50,
			MaxMS:    100,
			StepsMin: 20,
			StepsMax: 40,
		},
		AntiDetection: AntiDetectionConfig{
			TLSFingerprint:    false,
			CookieRotation:    false,
			UserAgentRotation: false,
			ViewportRandomize: false,
		},
	}

	// Carrega do YAML se existir (Premium features)
	if humanized, ok := cfg.Get("captcha.humanized_movement").(map[string]interface{}); ok {
		if enabled, ok := humanized["enabled"].(bool); ok {
			captchaCfg.Movement.Enabled = enabled
		}
		if bezier, ok := humanized["bezier_curves"].(bool); ok {
			captchaCfg.Movement.Bezier = bezier
		}
		if overshoot, ok := humanized["overshoot"].(bool); ok {
			captchaCfg.Movement.Overshoot = overshoot
		}
		if pauses, ok := humanized["micro_pauses"].(bool); ok {
			captchaCfg.Movement.MicroPauses = pauses
		}
		if tremor, ok := humanized["tremor"].(bool); ok {
			captchaCfg.Movement.Tremor = tremor
		}
	}

	if delays, ok := cfg.Get("captcha.delays").(map[string]interface{}); ok {
		if delayType, ok := delays["type"].(string); ok {
			captchaCfg.Delays.Type = delayType
		}
		if minMS, ok := delays["min_ms"].(int); ok {
			captchaCfg.Delays.MinMS = minMS
		}
		if maxMS, ok := delays["max_ms"].(int); ok {
			captchaCfg.Delays.MaxMS = maxMS
		}
		if stepsMin, ok := delays["steps_min"].(int); ok {
			captchaCfg.Delays.StepsMin = stepsMin
		}
		if stepsMax, ok := delays["steps_max"].(int); ok {
			captchaCfg.Delays.StepsMax = stepsMax
		}
	}

	if antiDetect, ok := cfg.Get("captcha.anti_detection").(map[string]interface{}); ok {
		if tls, ok := antiDetect["tls_fingerprint"].(bool); ok {
			captchaCfg.AntiDetection.TLSFingerprint = tls
		}
		if cookies, ok := antiDetect["cookie_rotation"].(bool); ok {
			captchaCfg.AntiDetection.CookieRotation = cookies
		}
		if ua, ok := antiDetect["user_agent_rotation"].(bool); ok {
			captchaCfg.AntiDetection.UserAgentRotation = ua
		}
		if viewport, ok := antiDetect["viewport_randomization"].(bool); ok {
			captchaCfg.AntiDetection.ViewportRandomize = viewport
		}
	}

	return captchaCfg
}

// IsPremium verifica se está usando features premium
func (c *CaptchaConfig) IsPremium() bool {
	return c.Movement.Enabled ||
		c.Delays.Type == "gaussian" ||
		c.AntiDetection.TLSFingerprint
}

// PrintConfig mostra configuração ativa
func (c *CaptchaConfig) PrintConfig() {
	if c.IsPremium() {
		fmt.Println("[Config] Modo PREMIUM ativado")
		fmt.Printf("  Movimento humanizado: %v\n", c.Movement.Enabled)
		fmt.Printf("  Delays: %s\n", c.Delays.Type)
		fmt.Printf("  Anti-detecção: %v\n", c.AntiDetection.TLSFingerprint)
	} else {
		fmt.Println("[Config] Modo OPEN SOURCE (básico)")
		fmt.Println("  Para ativar features premium, edite config.yaml")
	}
}
