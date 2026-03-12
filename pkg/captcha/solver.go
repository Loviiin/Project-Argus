package captcha

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	_ "golang.org/x/image/webp"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	modelInputSize = 224
	modelChannels  = 3
	tensorLength   = 1 * modelChannels * modelInputSize * modelInputSize // 150528
)

// Solver executa inferência ONNX para resolver captchas de rotação.
// Thread-safe: pode ser chamado de múltiplas goroutines.
type Solver struct {
	session *ort.DynamicAdvancedSession
	mu      sync.Mutex
}

// NewSolver inicializa o ONNX Runtime e cria uma sessão com o modelo.
// modelPath é o caminho absoluto para o arquivo .onnx.
// sharedLibPath é o caminho para libonnxruntime.so (pode ser "" para usar o default).
func NewSolver(modelPath string, sharedLibPath string) (*Solver, error) {
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("modelo não encontrado em '%s': %w", modelPath, err)
	}

	if sharedLibPath == "" {
		sharedLibPath = "libonnxruntime.so"
	}
	ort.SetSharedLibraryPath(sharedLibPath)

	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("erro inicializando ONNX Runtime: %w", err)
	}

	inputNames := []string{"outer_img", "inner_img"}
	outputNames := []string{"angle_deg"}

	session, err := ort.NewDynamicAdvancedSession(
		modelPath,
		inputNames,
		outputNames,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("erro criando sessão ONNX: %w", err)
	}

	return &Solver{session: session}, nil
}

// Close libera os recursos do solver e do ONNX Runtime.
func (s *Solver) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session != nil {
		if err := s.session.Destroy(); err != nil {
			return fmt.Errorf("erro destruindo sessão: %w", err)
		}
	}
	return ort.DestroyEnvironment()
}

// PredictBytes recebe as imagens outer e inner como bytes crus (PNG/JPEG/WebP)
// e retorna o ângulo previsto em graus. Este é o método primário de inferência.
func (s *Solver) PredictBytes(outerRaw, innerRaw []byte) (float32, error) {
	outerData, err := preprocessImageBytes(outerRaw)
	if err != nil {
		return 0, fmt.Errorf("erro pré-processando outer: %w", err)
	}

	innerData, err := preprocessImageBytes(innerRaw)
	if err != nil {
		return 0, fmt.Errorf("erro pré-processando inner: %w", err)
	}

	return s.runInference(outerData, innerData)
}

// Predict recebe as imagens outer e inner como strings base64 (sem prefixo data:)
// e retorna o ângulo previsto em graus. Decodifica para bytes e delega a PredictBytes.
func (s *Solver) Predict(outerB64, innerB64 string) (float32, error) {
	outerRaw, err := decodeBase64Input(outerB64)
	if err != nil {
		return 0, fmt.Errorf("erro decodificando outer base64: %w", err)
	}
	innerRaw, err := decodeBase64Input(innerB64)
	if err != nil {
		return 0, fmt.Errorf("erro decodificando inner base64: %w", err)
	}
	return s.PredictBytes(outerRaw, innerRaw)
}

// runInference cria tensores, executa a sessão ONNX e retorna o ângulo.
func (s *Solver) runInference(outerData, innerData []float32) (float32, error) {
	shape := ort.NewShape(1, modelChannels, modelInputSize, modelInputSize)

	outerTensor, err := ort.NewTensor(shape, outerData)
	if err != nil {
		return 0, fmt.Errorf("erro criando tensor outer: %w", err)
	}
	defer outerTensor.Destroy()

	innerTensor, err := ort.NewTensor(shape, innerData)
	if err != nil {
		return 0, fmt.Errorf("erro criando tensor inner: %w", err)
	}
	defer innerTensor.Destroy()

	outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(1))
	if err != nil {
		return 0, fmt.Errorf("erro criando tensor de saída: %w", err)
	}
	defer outputTensor.Destroy()

	s.mu.Lock()
	err = s.session.Run(
		[]ort.Value{outerTensor, innerTensor},
		[]ort.Value{outputTensor},
	)
	s.mu.Unlock()
	if err != nil {
		return 0, fmt.Errorf("erro executando inferência: %w", err)
	}

	output := outputTensor.GetData()
	if len(output) == 0 {
		return 0, fmt.Errorf("tensor de saída vazio")
	}

	return output[0], nil
}

// preprocessImageBytes decodifica uma imagem de bytes crus, redimensiona para 224×224,
// normaliza pixels para [0, 1] e retorna um slice no formato NCHW.
// Apenas divide por 255.0 — o modelo ONNX já contém normalização ImageNet e polar unroll.
func preprocessImageBytes(raw []byte) ([]float32, error) {
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("erro decodificando imagem: %w", err)
	}

	resized := resizeBilinear(img, modelInputSize, modelInputSize)

	// Formato NCHW: todos R, depois todos G, depois todos B
	data := make([]float32, tensorLength)
	planeSize := modelInputSize * modelInputSize

	for y := 0; y < modelInputSize; y++ {
		for x := 0; x < modelInputSize; x++ {
			r, g, b, _ := resized.At(x, y).RGBA()
			idx := y*modelInputSize + x
			data[idx] = float32(r>>8) / 255.0             // R plane
			data[planeSize+idx] = float32(g>>8) / 255.0   // G plane
			data[2*planeSize+idx] = float32(b>>8) / 255.0 // B plane
		}
	}

	return data, nil
}

// preprocessImage decodifica uma imagem base64, converte para bytes, e delega
// a preprocessImageBytes.
func preprocessImage(b64 string) ([]float32, error) {
	raw, err := decodeBase64Input(b64)
	if err != nil {
		return nil, err
	}
	return preprocessImageBytes(raw)
}

// decodeBase64Input decodifica uma string base64 (com ou sem data URI prefix).
func decodeBase64Input(b64 string) ([]byte, error) {
	if idx := strings.Index(b64, ","); idx >= 0 {
		b64 = b64[idx+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("erro decodificando base64: %w", err)
		}
	}
	return raw, nil
}

// resizeBilinear redimensiona uma imagem usando interpolação bilinear.
func resizeBilinear(src image.Image, width, height int) image.Image {
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	for dstY := 0; dstY < height; dstY++ {
		for dstX := 0; dstX < width; dstX++ {
			// Coordenadas no espaço da imagem fonte
			srcXf := (float64(dstX)+0.5)*float64(srcW)/float64(width) - 0.5
			srcYf := (float64(dstY)+0.5)*float64(srcH)/float64(height) - 0.5

			x0 := int(srcXf)
			y0 := int(srcYf)
			x1 := x0 + 1
			y1 := y0 + 1

			// Clamp aos limites
			if x0 < 0 {
				x0 = 0
			}
			if y0 < 0 {
				y0 = 0
			}
			if x1 >= srcW {
				x1 = srcW - 1
			}
			if y1 >= srcH {
				y1 = srcH - 1
			}

			// Pesos de interpolação
			dx := srcXf - float64(x0)
			dy := srcYf - float64(y0)
			if dx < 0 {
				dx = 0
			}
			if dy < 0 {
				dy = 0
			}

			// 4 pixels vizinhos
			r00, g00, b00, a00 := src.At(x0+srcBounds.Min.X, y0+srcBounds.Min.Y).RGBA()
			r10, g10, b10, a10 := src.At(x1+srcBounds.Min.X, y0+srcBounds.Min.Y).RGBA()
			r01, g01, b01, a01 := src.At(x0+srcBounds.Min.X, y1+srcBounds.Min.Y).RGBA()
			r11, g11, b11, a11 := src.At(x1+srcBounds.Min.X, y1+srcBounds.Min.Y).RGBA()

			// Interpolação bilinear
			lerp := func(v00, v10, v01, v11 uint32) uint8 {
				top := float64(v00)*(1-dx) + float64(v10)*dx
				bot := float64(v01)*(1-dx) + float64(v11)*dx
				val := top*(1-dy) + bot*dy
				return uint8(val / 256)
			}

			r := lerp(r00, r10, r01, r11)
			g := lerp(g00, g10, g01, g11)
			b := lerp(b00, b10, b01, b11)
			a := lerp(a00, a10, a01, a11)

			dst.SetRGBA(dstX, dstY, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}

	return dst
}

// DownloadImageBytes baixa uma imagem por URL e retorna os bytes crus.
func DownloadImageBytes(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("erro baixando imagem: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d ao baixar imagem", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro lendo body: %w", err)
	}

	return data, nil
}
