package captcha

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

const datasetDir = "/workspaces/Project-Argus/services/discovery/dataset/rotation_captcha"
const modelPath = "/workspaces/Project-Argus/argus_v6_csl_fp32.onnx"
const runtimePath = "/usr/lib/libonnxruntime.so"

type labelJSON struct {
	ID            string  `json:"id"`
	Angle         float64 `json:"angle"`
	RawPixels     float64 `json:"raw_pixels"`
	SlidebarWidth float64 `json:"slidebar_width"`
	IconWidth     float64 `json:"icon_width"`
}

// angularError calcula o menor erro angular considerando wraparound em 360°.
func angularError(predicted, actual float64) float64 {
	diff := math.Mod(math.Abs(predicted-actual), 360)
	if diff > 180 {
		diff = 360 - diff
	}
	return diff
}

// TestSolverWithRealDataset carrega imagens reais do Shadow Collector,
// roda inferência ONNX e compara com o ângulo real medido pelo humano.
func TestSolverWithRealDataset(t *testing.T) {
	if _, err := os.Stat(modelPath); err != nil {
		t.Skipf("Modelo não encontrado: %s", modelPath)
	}
	if _, err := os.Stat(runtimePath); err != nil {
		t.Skipf("libonnxruntime.so não encontrada")
	}
	if _, err := os.Stat(datasetDir); err != nil {
		t.Skipf("Dataset não encontrado: %s", datasetDir)
	}

	solver, err := NewSolver(modelPath, runtimePath)
	if err != nil {
		t.Fatalf("NewSolver: %v", err)
	}
	defer solver.Close()

	labels, err := filepath.Glob(filepath.Join(datasetDir, "*_label.json"))
	if err != nil || len(labels) == 0 {
		t.Fatalf("Nenhum label encontrado no dataset")
	}

	maxSamples := 30
	if len(labels) < maxSamples {
		maxSamples = len(labels)
	}

	var totalError float64
	var passed, failed, skipped int
	threshold := 40.0 // tolerância de erro em graus

	fmt.Printf("\n🧪 Testando solver ONNX com %d amostras reais do dataset\n", maxSamples)
	fmt.Println("─────────────────────────────────────────────────────────────────")
	fmt.Printf("%-12s │ %8s │ %8s │ %8s │ %s\n", "ID", "Real", "Predito", "Erro", "Status")
	fmt.Println("─────────────────────────────────────────────────────────────────")

	for i := 0; i < maxSamples; i++ {
		labelPath := labels[i]

		data, err := os.ReadFile(labelPath)
		if err != nil {
			t.Logf("Erro lendo label %s: %v", labelPath, err)
			skipped++
			continue
		}

		var label labelJSON
		if err := json.Unmarshal(data, &label); err != nil {
			t.Logf("Erro parseando label %s: %v", labelPath, err)
			skipped++
			continue
		}

		outerPath := filepath.Join(datasetDir, label.ID+"_outer.jpg")
		innerPath := filepath.Join(datasetDir, label.ID+"_inner.jpg")

		if _, err := os.Stat(outerPath); err != nil {
			skipped++
			continue
		}
		if _, err := os.Stat(innerPath); err != nil {
			skipped++
			continue
		}

		outerBytes, err := os.ReadFile(outerPath)
		if err != nil {
			skipped++
			continue
		}
		innerBytes, err := os.ReadFile(innerPath)
		if err != nil {
			skipped++
			continue
		}

		// Usa PredictBytes diretamente com bytes crus (sem base64 roundtrip).
		anglePred, err := solver.PredictBytes(outerBytes, innerBytes)
		if err != nil {
			t.Logf("Erro Predict para %s: %v", label.ID, err)
			skipped++
			continue
		}

		// Normaliza ambos para [0, 360)
		predNorm := math.Mod(float64(anglePred), 360)
		if predNorm < 0 {
			predNorm += 360
		}
		realAngle := label.Angle

		errDeg := angularError(predNorm, realAngle)
		totalError += errDeg

		status := "✅"
		if errDeg > threshold {
			status = "❌"
			failed++
		} else {
			passed++
		}

		// Simula a conversão para pixels também
		pixels := AngleToPixels(anglePred, label.SlidebarWidth, label.IconWidth)
		realPixels := label.RawPixels
		pixelErr := math.Abs(pixels - realPixels)

		shortID := label.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}

		fmt.Printf("%-12s │ %7.1f° │ %7.1f° │ %6.1f°  │ %s  (px: pred=%.0f real=%.0f err=%.0fpx)\n",
			shortID, realAngle, predNorm, errDeg, status, pixels, realPixels, pixelErr)
	}

	fmt.Println("─────────────────────────────────────────────────────────────────")

	evaluated := passed + failed
	if evaluated == 0 {
		t.Fatal("Nenhuma amostra avaliada")
	}

	mae := totalError / float64(evaluated)
	accuracy := float64(passed) / float64(evaluated) * 100

	fmt.Printf("\n📊 RESULTADO:\n")
	fmt.Printf("   Amostras: %d avaliadas, %d puladas\n", evaluated, skipped)
	fmt.Printf("   Dentro de ±%.0f°: %d/%d (%.1f%%)\n", threshold, passed, evaluated, accuracy)
	fmt.Printf("   MAE (Erro Médio): %.2f°\n", mae)
	fmt.Printf("   ✅ Passed: %d  ❌ Failed: %d\n\n", passed, failed)

	if accuracy < 50 {
		t.Errorf("Acurácia muito baixa: %.1f%% (mínimo esperado: 50%%)", accuracy)
	}

	// Teste da pipeline completa: angle -> pixels
	t.Run("pipeline_angle_to_pixels", func(t *testing.T) {
		// Usa a primeira amostra válida para validar pipeline
		data, _ := os.ReadFile(labels[0])
		var label labelJSON
		json.Unmarshal(data, &label)

		if label.SlidebarWidth <= label.IconWidth {
			t.Skip("Dimensões inválidas no label")
		}

		outerBytes, _ := os.ReadFile(filepath.Join(datasetDir, label.ID+"_outer.jpg"))
		innerBytes, _ := os.ReadFile(filepath.Join(datasetDir, label.ID+"_inner.jpg"))

		angle, err := solver.PredictBytes(outerBytes, innerBytes)
		if err != nil {
			t.Fatalf("PredictBytes: %v", err)
		}

		pixels := AngleToPixels(angle, label.SlidebarWidth, label.IconWidth)
		t.Logf("Pipeline: ângulo=%.2f° → pixels=%.2f (real=%.2f, track=%.0f, knob=%.0f)",
			angle, pixels, label.RawPixels, label.SlidebarWidth, label.IconWidth)

		if pixels < 0 || pixels > label.SlidebarWidth {
			t.Errorf("Pixels fora do range: %.2f (max=%.0f)", pixels, label.SlidebarWidth)
		}
	})
}

// TestSolverDeterminism verifica que chamadas repetidas com a mesma imagem
// retornam o mesmo resultado (modelo é determinístico).
func TestSolverDeterminism(t *testing.T) {
	if _, err := os.Stat(modelPath); err != nil {
		t.Skipf("Modelo não encontrado")
	}
	if _, err := os.Stat(runtimePath); err != nil {
		t.Skipf("libonnxruntime.so não encontrada")
	}

	labels, _ := filepath.Glob(filepath.Join(datasetDir, "*_label.json"))
	if len(labels) == 0 {
		t.Skip("Dataset vazio")
	}

	solver, err := NewSolver(modelPath, runtimePath)
	if err != nil {
		t.Fatalf("NewSolver: %v", err)
	}
	defer solver.Close()

	data, _ := os.ReadFile(labels[0])
	var label labelJSON
	json.Unmarshal(data, &label)

	outerBytes, _ := os.ReadFile(filepath.Join(datasetDir, label.ID+"_outer.jpg"))
	innerBytes, _ := os.ReadFile(filepath.Join(datasetDir, label.ID+"_inner.jpg"))

	results := make([]float32, 5)
	for i := range results {
		results[i], err = solver.PredictBytes(outerBytes, innerBytes)
		if err != nil {
			t.Fatalf("PredictBytes #%d: %v", i, err)
		}
	}

	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			t.Errorf("Resultado não-determinístico: run[0]=%.4f, run[%d]=%.4f", results[0], i, results[i])
		}
	}
	t.Logf("✅ Modelo determinístico: 5 runs → %.4f°", results[0])
}
