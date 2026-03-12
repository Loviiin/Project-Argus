package captcha

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// createTestImageB64 cria uma imagem PNG de teste (colorida) e retorna como base64.
func createTestImageB64(width, height int, c color.RGBA) string {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestPreprocessImage(t *testing.T) {
	b64 := createTestImageB64(100, 100, color.RGBA{R: 255, G: 128, B: 0, A: 255})

	data, err := preprocessImage(b64)
	if err != nil {
		t.Fatalf("preprocessImage() error: %v", err)
	}

	expectedLen := 1 * 3 * 224 * 224
	if len(data) != expectedLen {
		t.Fatalf("preprocessImage() output length = %d, want %d", len(data), expectedLen)
	}

	// Verifica formato NCHW: todos os R ficam no primeiro plano
	planeSize := 224 * 224
	rVal := data[0]           // primeiro pixel do plano R
	gVal := data[planeSize]   // primeiro pixel do plano G
	bVal := data[2*planeSize] // primeiro pixel do plano B

	// Imagem toda vermelha-laranja: R≈1.0, G≈0.5, B≈0.0
	if rVal < 0.9 || rVal > 1.01 {
		t.Errorf("R channel = %f, want ~1.0", rVal)
	}
	if gVal < 0.45 || gVal > 0.55 {
		t.Errorf("G channel = %f, want ~0.5", gVal)
	}
	if bVal > 0.01 {
		t.Errorf("B channel = %f, want ~0.0", bVal)
	}

	// Verifica que valores estão entre 0 e 1
	for i, v := range data {
		if v < 0 || v > 1.001 {
			t.Fatalf("data[%d] = %f, fora do range [0, 1]", i, v)
		}
	}
}

func TestPreprocessImageWithDataURI(t *testing.T) {
	raw := createTestImageB64(50, 50, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	dataURI := "data:image/png;base64," + raw

	data, err := preprocessImage(dataURI)
	if err != nil {
		t.Fatalf("preprocessImage() com data URI error: %v", err)
	}

	if len(data) != tensorLength {
		t.Fatalf("output length = %d, want %d", len(data), tensorLength)
	}
}

func TestResizeBilinear(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 100, G: 200, B: 50, A: 255})
		}
	}

	dst := resizeBilinear(src, 224, 224)
	bounds := dst.Bounds()

	if bounds.Dx() != 224 || bounds.Dy() != 224 {
		t.Errorf("resized image = %dx%d, want 224x224", bounds.Dx(), bounds.Dy())
	}
}

func TestSolverIntegration(t *testing.T) {
	modelPath := "/workspaces/Project-Argus/argus_v6_csl_fp32.onnx"
	if _, err := os.Stat(modelPath); err != nil {
		t.Skipf("Modelo não encontrado em %s — pulando teste de integração", modelPath)
	}

	runtimePath := "/usr/lib/libonnxruntime.so"
	if _, err := os.Stat(runtimePath); err != nil {
		t.Skipf("libonnxruntime.so não encontrada — pulando teste de integração")
	}

	solver, err := NewSolver(modelPath, runtimePath)
	if err != nil {
		t.Fatalf("NewSolver() error: %v", err)
	}
	defer solver.Close()

	// Imagem outer (fundo azul) e inner (fundo vermelho) sintéticas
	outerB64 := createTestImageB64(224, 224, color.RGBA{R: 50, G: 100, B: 200, A: 255})
	innerB64 := createTestImageB64(224, 224, color.RGBA{R: 200, G: 50, B: 50, A: 255})

	angle, err := solver.Predict(outerB64, innerB64)
	if err != nil {
		t.Fatalf("Predict() error: %v", err)
	}

	t.Logf("✅ Predict() retornou: %.4f graus", angle)

	// O modelo retorna entre -180 e 180
	if angle < -180 || angle > 180 {
		t.Errorf("Ângulo fora do range esperado: %f (esperado [-180, 180])", angle)
	}
}
