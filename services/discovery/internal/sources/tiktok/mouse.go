package tiktok

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// Variáveis para ruído Perlin simplificado (tremor mais natural)
var (
	perlinSeed = rand.Float64() * 1000
	perlinTime = 0.0
	lastMouseX = 0.0
	lastMouseY = 0.0
)

// DragSlider arrasta o slider do captcha simulando movimento humano
// Usa curvas de Bézier, aceleração variável e micro-correções para evitar detecção
func DragSlider(page *rod.Page, slider *rod.Element, distanceX float64) error {
	// Obtém a posição atual do slider usando Shape()
	shape, err := slider.Shape()
	if err != nil {
		return fmt.Errorf("erro obtendo posição do slider: %w", err)
	}

	// Usa o primeiro quad (retângulo) da shape
	if len(shape.Quads) == 0 {
		return fmt.Errorf("slider não tem dimensões válidas")
	}

	quad := shape.Quads[0]
	// Calcula o centro do elemento
	startX := (quad[0] + quad[2]) / 2
	startY := (quad[1] + quad[5]) / 2

	// Ponto final com pequeno desvio vertical (humanos não movem perfeitamente reto)
	endX := startX + distanceX
	endY := startY + randomFloat(-3, 3)

	// Move o mouse para o início do slider com approach natural
	if err := moveMouseToElement(page, startX, startY); err != nil {
		return err
	}

	// Pausa de "reconhecimento" - humano olha para o elemento antes de clicar
	time.Sleep(time.Duration(randomInt(150, 400)) * time.Millisecond)

	// Pequeno movimento de ajuste (humanos fazem micro-ajustes antes de clicar)
	adjustX := startX + randomFloat(-2, 2)
	adjustY := startY + randomFloat(-2, 2)
	page.Mouse.MoveLinear(proto.Point{X: adjustX, Y: adjustY}, 1)
	time.Sleep(time.Duration(randomInt(30, 80)) * time.Millisecond)

	// Volta para a posição exata do centro do slider
	page.Mouse.MoveLinear(proto.Point{X: startX, Y: startY}, 1)
	time.Sleep(time.Duration(randomInt(50, 120)) * time.Millisecond)

	// CRÍTICO: Garante que o mouse está sobre o elemento antes de pressionar
	// Dispara um evento de hover para ativar o elemento
	if err := slider.Hover(); err != nil {
		fmt.Printf("[Mouse] ⚠️  Hover falhou: %v (continuando...)\n", err)
	}
	time.Sleep(time.Duration(randomInt(50, 100)) * time.Millisecond)

	// Pressiona o botão do mouse diretamente no elemento
	// Isso garante que o evento mousedown seja disparado no elemento correto
	if err := page.Mouse.Down("left", 1); err != nil {
		return fmt.Errorf("erro pressionando mouse: %w", err)
	}

	// Pequena pausa após pressionar (humanos não começam a arrastar imediatamente)
	time.Sleep(time.Duration(randomInt(80, 200)) * time.Millisecond)

	// Arrasta usando movimento humano avançado
	if err := dragWithHumanMovement(page, startX, startY, endX, endY); err != nil {
		page.Mouse.Up("left", 1) // Garante que solta o mouse mesmo se houver erro
		return err
	}

	// MELHORIA 4: Pausa antes de soltar o mouse (humanos hesitam para verificar posição)
	pauseBeforeRelease := time.Duration(randomInt(100, 300)) * time.Millisecond
	time.Sleep(pauseBeforeRelease)

	// Micro-ajuste final (humanos verificam se está na posição correta)
	if rand.Float64() > 0.4 { // 60% de chance
		finalAdjustX := endX + randomFloat(-1.5, 1.5)
		page.Mouse.MoveLinear(proto.Point{X: finalAdjustX, Y: endY}, 1)
		time.Sleep(time.Duration(randomInt(40, 100)) * time.Millisecond)
		page.Mouse.MoveLinear(proto.Point{X: endX, Y: endY}, 1)
		time.Sleep(time.Duration(randomInt(30, 80)) * time.Millisecond)
	}

	// Solta o botão do mouse
	if err := page.Mouse.Up("left", 1); err != nil {
		return fmt.Errorf("erro soltando mouse: %w", err)
	}

	// Pausa pós-soltar (humanos não movem mouse imediatamente após soltar)
	time.Sleep(time.Duration(randomInt(100, 250)) * time.Millisecond)
	return nil
}

// moveMouseToElement move o mouse até um elemento usando approach natural
// MELHORIA 2: Posição inicial não é mais sempre 0,0
func moveMouseToElement(page *rod.Page, targetX, targetY float64) error {
	// Usa última posição conhecida ou estima uma posição inicial realista
	startX := lastMouseX
	startY := lastMouseY

	// Se nunca movemos o mouse, assume posição inicial aleatória na viewport
	if startX == 0 && startY == 0 {
		startX = randomFloat(100, 400)
		startY = randomFloat(100, 300)
		// Move para posição inicial
		page.Mouse.MoveLinear(proto.Point{X: startX, Y: startY}, 1)
		time.Sleep(time.Duration(randomInt(50, 100)) * time.Millisecond)
	}

	distance := math.Sqrt(math.Pow(targetX-startX, 2) + math.Pow(targetY-startY, 2))
	steps := int(math.Max(25, math.Min(60, distance/10)))

	// Pontos de controle para curva de Bézier (cria trajetória natural)
	cp1X := startX + (targetX-startX)*randomFloat(0.2, 0.4) + randomFloat(-20, 20)
	cp1Y := startY + (targetY-startY)*randomFloat(0.2, 0.4) + randomFloat(-20, 20)
	cp2X := startX + (targetX-startX)*randomFloat(0.6, 0.8) + randomFloat(-10, 10)
	cp2Y := startY + (targetY-startY)*randomFloat(0.6, 0.8) + randomFloat(-10, 10)

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)

		// Curva de Bézier cúbica
		x := cubicBezier(t, startX, cp1X, cp2X, targetX)
		y := cubicBezier(t, startY, cp1Y, cp2Y, targetY)

		// Tremor humano usando ruído Perlin simplificado
		tremor := getHumanTremor()
		x += tremor * 1.5
		y += tremor * 1.0

		if err := page.Mouse.MoveLinear(proto.Point{X: x, Y: y}, 1); err != nil {
			return fmt.Errorf("erro movendo mouse: %w", err)
		}

		// Velocidade variável com easing
		delay := calculateApproachDelay(t)
		time.Sleep(time.Duration(delay) * time.Millisecond)

		// Ocasional hesitação (humanos às vezes pausam durante movimento)
		if rand.Float64() < 0.05 { // 5% de chance
			time.Sleep(time.Duration(randomInt(30, 80)) * time.Millisecond)
		}
	}

	// Atualiza última posição
	lastMouseX = targetX
	lastMouseY = targetY

	return nil
}

// dragWithHumanMovement arrasta o mouse de um ponto a outro simulando movimento humano
// Implementa Lei de Fitts, curva de Bézier, easing functions, overshoot e micro-correções
func dragWithHumanMovement(page *rod.Page, startX, startY, endX, endY float64) error {
	// Lei de Fitts: MT = a + b * log2(D/W + 1)
	distance := math.Abs(endX - startX)
	targetWidth := 50.0

	// Calcula steps com Fitts adaptado
	fittsIndex := math.Log2(distance/targetWidth + 1)
	baseSteps := 100.0
	steps := int(baseSteps + fittsIndex*25)

	// Range razoável
	if steps < 100 {
		steps = 100
	}
	if steps > 200 {
		steps = 200
	}

	// Pontos de controle para curva natural
	cp1X := startX + (endX-startX)*randomFloat(0.15, 0.35)
	cp1Y := startY + randomFloat(-12, 12)
	cp2X := startX + (endX-startX)*randomFloat(0.65, 0.85)
	cp2Y := startY + randomFloat(-12, 12)

	// MELHORIA 5: Micro-correções durante o arrasto
	correctionPoints := generateCorrectionPoints(steps)

	var currentX, currentY float64

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)

		// Curva de Bézier cúbica base
		x := cubicBezier(t, startX, cp1X, cp2X, endX)
		y := cubicBezier(t, startY, cp1Y, cp2Y, endY)

		// Tremor humano (mais pronunciado durante arrasto)
		tremor := getHumanTremor()
		x += tremor * 2.5
		y += tremor * 1.8

		// MELHORIA 5: Aplica micro-correções em pontos específicos
		if isInCorrectionZone(i, correctionPoints) {
			// Pequeno desvio seguido de correção
			deviationX := x + randomFloat(-4, 4)
			deviationY := y + randomFloat(-3, 3)

			page.Mouse.MoveLinear(proto.Point{X: deviationX, Y: deviationY}, 1)
			time.Sleep(time.Duration(randomInt(15, 35)) * time.Millisecond)

			// Correção de volta à trajetória
			x += randomFloat(-1, 1) // Não volta perfeitamente
		}

		if err := page.Mouse.MoveLinear(proto.Point{X: x, Y: y}, 1); err != nil {
			return fmt.Errorf("erro durante arrasto: %w", err)
		}

		currentX = x
		currentY = y

		// MELHORIA 3: Perfil de velocidade mais humano
		delay := calculateHumanDragDelay(t, distance)
		time.Sleep(time.Duration(delay) * time.Millisecond)

		// Micro-pausas ocasionais (hesitação humana)
		if i > 0 && i%randomInt(15, 30) == 0 {
			pauseDuration := randomInt(25, 70)
			time.Sleep(time.Duration(pauseDuration) * time.Millisecond)
		}

		// MELHORIA 2: Jitter ocasional (tremor maior esporádico)
		if rand.Float64() < 0.03 { // 3% de chance de jitter
			jitterX := currentX + randomFloat(-5, 5)
			jitterY := currentY + randomFloat(-3, 3)
			page.Mouse.MoveLinear(proto.Point{X: jitterX, Y: jitterY}, 1)
			time.Sleep(time.Duration(randomInt(10, 25)) * time.Millisecond)
		}
	}

	// Overshoot mais natural (varia baseado na velocidade)
	overshootChance := 0.65 // 65% base
	if distance > 150 {
		overshootChance = 0.80 // Mais overshoot em distâncias longas
	}

	if rand.Float64() < overshootChance {
		overshootAmount := randomFloat(2, 6) * (1 + distance/200) // Proporcional à distância
		overshootX := endX + overshootAmount
		// Vai além
		page.Mouse.MoveLinear(proto.Point{X: overshootX, Y: endY + randomFloat(-2, 2)}, 1)
		time.Sleep(time.Duration(randomInt(40, 90)) * time.Millisecond)

		// Correção gradual (não instantânea)
		midCorrectionX := endX + overshootAmount*0.4
		page.Mouse.MoveLinear(proto.Point{X: midCorrectionX, Y: endY}, 1)
		time.Sleep(time.Duration(randomInt(25, 50)) * time.Millisecond)

		// Posição final
		page.Mouse.MoveLinear(proto.Point{X: endX, Y: endY}, 1)
		time.Sleep(time.Duration(randomInt(20, 45)) * time.Millisecond)
	}

	// Atualiza última posição
	lastMouseX = endX
	lastMouseY = endY

	return nil
}

// generateCorrectionPoints gera pontos onde ocorrerão micro-correções
func generateCorrectionPoints(totalSteps int) []int {
	numCorrections := randomInt(2, 5)
	points := make([]int, numCorrections)

	for i := 0; i < numCorrections; i++ {
		// Distribui uniformemente mas com variação
		basePoint := (totalSteps / (numCorrections + 1)) * (i + 1)
		points[i] = basePoint + randomInt(-5, 5)
	}

	return points
}

// isInCorrectionZone verifica se o step atual está próximo de um ponto de correção
func isInCorrectionZone(step int, correctionPoints []int) bool {
	for _, cp := range correctionPoints {
		if math.Abs(float64(step-cp)) <= 2 {
			return rand.Float64() < 0.7 // 70% de chance de correção
		}
	}
	return false
}

// getHumanTremor retorna um valor de tremor baseado em ruído Perlin simplificado
func getHumanTremor() float64 {
	perlinTime += 0.1
	// Ruído Perlin simplificado usando seno com múltiplas frequências
	noise := math.Sin(perlinTime*0.7+perlinSeed) * 0.5
	noise += math.Sin(perlinTime*1.3+perlinSeed*1.5) * 0.3
	noise += math.Sin(perlinTime*2.7+perlinSeed*0.7) * 0.2
	return noise
}

// calculateApproachDelay calcula delay durante approach ao elemento
func calculateApproachDelay(t float64) int {
	// Perfil: rápido no início, desacelera ao se aproximar
	factor := easeOutQuad(t)

	minDelay := 8.0
	maxDelay := 25.0

	delay := maxDelay - factor*(maxDelay-minDelay)
	delay += randomFloat(-2, 2)

	return int(math.Max(delay, 5))
}

// calculateHumanDragDelay calcula delay com perfil humano durante arrasto
// MELHORIA 3: Perfil de aceleração mais natural
func calculateHumanDragDelay(t float64, distance float64) int {
	// Perfil em 3 fases:
	// 1. Início (0-20%): aceleração gradual
	// 2. Meio (20-80%): velocidade mais constante com variação
	// 3. Fim (80-100%): desaceleração para precisão

	var factor float64
	var baseMin, baseMax float64

	switch {
	case t < 0.2:
		// Fase de aceleração
		factor = easeInQuad(t / 0.2)
		baseMin = 25.0
		baseMax = 40.0
	case t < 0.8:
		// Fase constante (mais rápida)
		normalizedT := (t - 0.2) / 0.6
		factor = 0.5 + math.Sin(normalizedT*math.Pi)*0.3 // Pequena ondulação
		baseMin = 12.0
		baseMax = 22.0
	default:
		// Fase de desaceleração (precisão)
		normalizedT := (t - 0.8) / 0.2
		factor = 1.0 - easeOutQuad(normalizedT)
		baseMin = 20.0
		baseMax = 45.0
	}

	delay := baseMax - factor*(baseMax-baseMin)

	// Variação baseada na distância
	if distance > 200 {
		delay *= 0.85 // Mais rápido em distâncias longas
	}

	// Adiciona variação aleatória
	delay += randomFloat(-4, 4)

	return int(math.Max(delay, 8))
}

// MoveMouseSmooth move o mouse suavemente até uma posição (função auxiliar pública)
func MoveMouseSmooth(page *rod.Page, x, y float64) error {
	return moveMouseToElement(page, x, y)
}

// ClickWithDelay clica em um elemento com delay humano
func ClickWithDelay(element *rod.Element) error {
	// Delay antes do clique (pensando)
	time.Sleep(time.Duration(randomInt(120, 350)) * time.Millisecond)

	if err := element.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}

	// Delay após o clique (reação)
	time.Sleep(time.Duration(randomInt(80, 180)) * time.Millisecond)
	return nil
}

// cubicBezier calcula um ponto em uma curva de Bézier cúbica
func cubicBezier(t, p0, p1, p2, p3 float64) float64 {
	u := 1 - t
	tt := t * t
	uu := u * u
	uuu := uu * u
	ttt := tt * t

	return uuu*p0 + 3*uu*t*p1 + 3*u*tt*p2 + ttt*p3
}

// easeInQuad - aceleração quadrática
func easeInQuad(t float64) float64 {
	return t * t
}

// easeOutQuad - desaceleração quadrática
func easeOutQuad(t float64) float64 {
	return 1 - (1-t)*(1-t)
}

// easeInOutCubic implementa a função de easing cúbica (perfil de velocidade em sino)
func easeInOutCubic(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	return 1 - math.Pow(-2*t+2, 3)/2
}

// easeOutQuint implementa a função de easing quíntica (desaceleração forte)
func easeOutQuint(t float64) float64 {
	return 1 - math.Pow(1-t, 5)
}

// randomFloat retorna um número float64 aleatório entre min e max
func randomFloat(min, max float64) float64 {
	return min + rand.Float64()*(max-min)
}

// randomInt retorna um número inteiro aleatório entre min e max (inclusive)
func randomInt(min, max int) int {
	if min >= max {
		return min
	}
	return min + rand.IntN(max-min+1)
}
