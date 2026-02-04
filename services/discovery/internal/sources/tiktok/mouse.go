package tiktok

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

var captchaConfig CaptchaConfig

func init() {
	rand.Seed(time.Now().UnixNano())
	captchaConfig = LoadCaptchaConfig()
	captchaConfig.PrintConfig()
}

func DragSlider(page *rod.Page, slider *rod.Element, distanceX float64) error {
	shape, err := slider.Shape()
	if err != nil {
		return fmt.Errorf("erro obtendo posição do slider: %w", err)
	}

	if len(shape.Quads) == 0 {
		return fmt.Errorf("slider não tem dimensões válidas")
	}

	quad := shape.Quads[0]
	startX := (quad[0] + quad[2]) / 2
	startY := (quad[1] + quad[5]) / 2

	endX := startX + distanceX
	endY := startY + randomFloat(-5, 5)

	fmt.Printf(" Iniciando arrasto de (%.2f, %.2f) para (%.2f, %.2f)\n",
		startX, startY, endX, endY)

	// Move o mouse para o início do slider
	if err := moveMouseHuman(page, startX, startY); err != nil {
		return err
	}

	time.Sleep(time.Duration(randomInt(100, 300)) * time.Millisecond)

	// Pressiona o botão do mouse
	if err := page.Mouse.Down("left", 1); err != nil {
		return fmt.Errorf("erro pressionando mouse: %w", err)
	}

	time.Sleep(time.Duration(randomInt(50, 150)) * time.Millisecond)

	// OPEN CORE: Escolhe modo básico ou premium
	if captchaConfig.Movement.Enabled {
		fmt.Println("[PREMIUM] Usando movimento humanizado avançado")
		if err := dragWithHumanMovement(page, startX, startY, endX, endY); err != nil {
			page.Mouse.Up("left", 1)
			return err
		}
	} else {
		fmt.Println("[OPEN] Usando movimento linear básico")
		if err := dragWithBasicMovement(page, startX, startY, endX, endY); err != nil {
			page.Mouse.Up("left", 1)
			return err
		}
	}

	time.Sleep(time.Duration(randomInt(50, 100)) * time.Millisecond)

	// Solta o botão do mouse
	if err := page.Mouse.Up("left", 1); err != nil {
		return fmt.Errorf("erro soltando mouse: %w", err)
	}

	fmt.Println(" Arrasto concluído com sucesso")
	return nil
}

func moveMouseHuman(page *rod.Page, targetX, targetY float64) error {
	startX, startY := 0.0, 0.0
	steps := randomInt(20, 40)

	cp1X := startX + (targetX-startX)*randomFloat(0.2, 0.4)
	cp1Y := startY + (targetY-startY)*randomFloat(-0.3, 0.3)
	cp2X := startX + (targetX-startX)*randomFloat(0.6, 0.8)
	cp2Y := startY + (targetY-startY)*randomFloat(-0.3, 0.3)

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)

		x := cubicBezier(t, startX, cp1X, cp2X, targetX)
		y := cubicBezier(t, startY, cp1Y, cp2Y, targetY)

		x += randomFloat(-1, 1)
		y += randomFloat(-1, 1)

		if err := page.Mouse.MoveLinear(proto.Point{X: x, Y: y}, 1); err != nil {
			return fmt.Errorf("erro movendo mouse: %w", err)
		}

		// Velocidade variável (aceleração e desaceleração)
		delay := calculateDelay(t)
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}

	return nil
}

// dragWithBasicMovement movimento linear simples (OPEN SOURCE)
func dragWithBasicMovement(page *rod.Page, startX, startY, endX, endY float64) error {
	// Configuração básica (open source)
	steps := randomInt(captchaConfig.Delays.StepsMin, captchaConfig.Delays.StepsMax)

	deltaX := (endX - startX) / float64(steps)
	deltaY := (endY - startY) / float64(steps)

	for i := 0; i <= steps; i++ {
		x := startX + deltaX*float64(i)
		y := startY + deltaY*float64(i)

		if err := page.Mouse.MoveLinear(proto.Point{X: x, Y: y}, 1); err != nil {
			return fmt.Errorf("erro durante arrasto: %w", err)
		}

		// Delay fixo (open source)
		delay := randomInt(captchaConfig.Delays.MinMS, captchaConfig.Delays.MaxMS)
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}

	return nil
}

// dragWithHumanMovement arrasta usando Bézier, overshoot, tremor (PREMIUM - CLOSED SOURCE)
func dragWithHumanMovement(page *rod.Page, startX, startY, endX, endY float64) error {
	// Lei de Fitts: MT = a + b * log2(D/W + 1)
	// Onde D = distância, W = largura do alvo
	// Calcula número de steps baseado na distância (mais longe = mais steps)
	distance := math.Abs(endX - startX)
	targetWidth := 50.0 // Largura aproximada do alvo (slider)

	// Fórmula de Fitts adaptada para calcular steps
	fittsIndex := math.Log2(distance/targetWidth + 1)
	baseSteps := 80.0
	steps := int(baseSteps + fittsIndex*20) // Mais distância = mais steps

	// Garante range razoável
	if steps < 80 {
		steps = 80
	}
	if steps > 150 {
		steps = 150
	}

	fmt.Printf(" Lei de Fitts: distância=%.1f, steps=%d, índice=%.2f\n",
		distance, steps, fittsIndex)

	// Pontos de controle para criar uma curva natural (mais variação)
	cp1X := startX + (endX-startX)*randomFloat(0.20, 0.40)
	cp1Y := startY + randomFloat(-15, 15) // Desvio vertical maior
	cp2X := startX + (endX-startX)*randomFloat(0.60, 0.80)
	cp2Y := startY + randomFloat(-15, 15)

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)

		// Curva de Bézier cúbica
		x := cubicBezier(t, startX, cp1X, cp2X, endX)
		y := cubicBezier(t, startY, cp1Y, cp2Y, endY)

		// Adiciona tremor humano (mais perceptível durante o arrasto)
		x += randomFloat(-3, 3)
		y += randomFloat(-2, 2)

		// Usa o Mouse.MoveLinear do Rod que dispara eventos apropriados
		if err := page.Mouse.MoveLinear(proto.Point{X: x, Y: y}, 1); err != nil {
			return fmt.Errorf("erro durante arrasto: %w", err)
		}

		// Velocidade variável - mais lento no começo e fim, mais rápido no meio
		delay := calculateDragDelay(t)
		time.Sleep(time.Duration(delay) * time.Millisecond)

		// Micro-pausas apenas se premium
		if captchaConfig.Movement.MicroPauses && i > 0 && i%randomInt(12, 20) == 0 {
			time.Sleep(time.Duration(randomInt(20, 50)) * time.Millisecond)
		}
	}

	// Overshoot apenas se premium
	if captchaConfig.Movement.Overshoot && randomFloat(0, 1) > 0.3 {
		overshootX := endX + randomFloat(3, 8)

		// Vai um pouco além
		page.Mouse.MoveLinear(proto.Point{X: overshootX, Y: endY}, 1)
		time.Sleep(time.Duration(randomInt(30, 60)) * time.Millisecond)

		// Corrige voltando ao ponto correto
		page.Mouse.MoveLinear(proto.Point{X: endX, Y: endY}, 1)
		time.Sleep(time.Duration(randomInt(20, 40)) * time.Millisecond)
	}

	return nil
}

// MoveMouseSmooth move o mouse suavemente até uma posição (função auxiliar pública)
func MoveMouseSmooth(page *rod.Page, x, y float64) error {
	return moveMouseHuman(page, x, y)
}

// ClickWithDelay clica em um elemento com delay humano
func ClickWithDelay(element *rod.Element) error {
	// Delay antes do clique
	time.Sleep(time.Duration(randomInt(100, 300)) * time.Millisecond)

	if err := element.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}

	// Delay após o clique
	time.Sleep(time.Duration(randomInt(100, 200)) * time.Millisecond)
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

// calculateDelay calcula o delay entre movimentos usando uma curva de aceleração
// Mais lento no início e fim, mais rápido no meio
func calculateDelay(t float64) int {
	// Usa ease-in-out-cubic para perfil de velocidade em sino
	factor := easeInOutCubic(t)

	minDelay := 5.0
	maxDelay := 20.0

	// Inverte: mais delay quando factor é baixo (início/fim)
	delay := maxDelay - (factor * (maxDelay - minDelay))

	return int(delay)
}

// calculateDragDelay calcula o delay durante o arrasto (mais lento que movimento normal)
// Implementa Lei de Fitts: tempo baseado em distância e precisão necessária
func calculateDragDelay(t float64) int {
	// Usa ease-out-quint para desaceleração suave (amortecimento)
	// Perfil de velocidade em sino: aceleração inicial, pico no meio, desaceleração no fim
	factor := easeOutQuint(t)

	// Delays mais longos = mais humano (humanos são lentos)
	minDelay := 15.0
	maxDelay := 35.0

	delay := maxDelay - (factor * (maxDelay - minDelay))

	// Adiciona variação aleatória maior
	delay += randomFloat(-3, 3)

	return int(math.Max(delay, 10))
}

// easeInOutCubic implementa a função de easing cúbica (perfil de velocidade em sino)
// Aceleração suave no início, desaceleração suave no fim
func easeInOutCubic(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	return 1 - math.Pow(-2*t+2, 3)/2
}

// easeOutQuint implementa a função de easing quíntica (desaceleração forte)
// Simula o amortecimento natural do movimento humano ao se aproximar do alvo
func easeOutQuint(t float64) float64 {
	return 1 - math.Pow(1-t, 5)
}

// randomFloat retorna um número float64 aleatório entre min e max
func randomFloat(min, max float64) float64 {
	return min + rand.Float64()*(max-min)
}

// randomInt retorna um número inteiro aleatório entre min e max (inclusive)
func randomInt(min, max int) int {
	return min + rand.Intn(max-min+1)
}
