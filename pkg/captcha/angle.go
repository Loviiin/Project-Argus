package captcha

import "math"

// AngleToPixels converte um ângulo (em graus, saída do modelo ONNX) para a distância
// em pixels que o slider deve ser arrastado.
//
// Física do TikTok: o slider gira os dois anéis em sentidos opostos.
// Arrastar o slider 100% gira outer -180° e inner +180° (relativo total = 360°).
// Portanto, o track inteiro (trackWidth - knobWidth) corresponde a 180° do modelo.
//
// Para ângulos negativos: -θ equivale a (180 - |θ|) no slider (mesma posição de encaixe).
//
//	pixels = (ânguloNormalizado / 180) × (trackWidth - knobWidth)
func AngleToPixels(angleDeg float32, trackWidth, knobWidth float64) float64 {
	// Normaliza para [-180, 180] via Remainder (simétrico, diferente de Mod)
	normalized := math.Remainder(float64(angleDeg), 360.0)

	// Ângulo negativo: -θ precisa somar 180° (o encaixe correto fica do "lado oposto").
	// Ex: -85° → -85+180 = 95° → slider em 52.8% do track.
	// ERRADO seria usar abs(-85) = 85° → slider em 47.2% (posição invertida).
	if normalized < 0 {
		normalized += 180.0
	}

	maxDistance := trackWidth - knobWidth
	if maxDistance <= 0 {
		return 0
	}

	return (normalized / 180.0) * maxDistance
}
