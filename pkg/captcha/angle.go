package captcha

import "math"

// AngleToPixels converte um ângulo (em graus, range -180 a 180) para a distância
// em pixels que o slider deve ser arrastado.
//
// Normaliza o ângulo para o espectro positivo [0, 360) usando math.Mod e calcula:
//
//	pixels = (ângulo / 360) × (trackWidth - knobWidth)
func AngleToPixels(angleDeg float32, trackWidth, knobWidth float64) float64 {
	// Normaliza para [0, 360)
	normalized := math.Mod(float64(angleDeg), 360.0)
	if normalized < 0 {
		normalized += 360.0
	}

	maxDistance := trackWidth - knobWidth
	if maxDistance <= 0 {
		return 0
	}

	return (normalized / 360.0) * maxDistance
}
