package tiktok

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// mlDataDir é o diretório base para armazenar dados de treinamento.
// O path é relativo à raiz do projeto (onde o binário é executado).
const mlDataDir = "../../ml/data/raw"

// CaptchaSample representa um sample coletado para treinamento futuro.
type CaptchaSample struct {
	Type      string   `json:"type"`      // "rotate" ou "slider"
	Timestamp int64    `json:"timestamp"` // Unix millis
	Manual    bool     `json:"manual"`    // true se resolvido manualmente pelo usuário
	ImageKeys []string `json:"image_keys"`
}

// SaveCaptchaSample salva as imagens do captcha em disco para treinamento futuro.
// Chamado automaticamente antes de entrar em modo manual, para que cada resolução
// humana gere um sample de treinamento rotulado "em aberto" (label gerada depois).
//
// Estrutura gerada:
//
//	services/ml/data/raw/<tipo>/<timestamp>_meta.json
//	services/ml/data/raw/<tipo>/<timestamp>_<nome>.png
func SaveCaptchaSample(captchaType string, images map[string]string, manual bool) {
	dir := filepath.Join(mlDataDir, captchaType)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("⚠️  [ML] Não foi possível criar diretório %s: %v\n", dir, err)
		return
	}

	ts := time.Now().UnixMilli()

	// Salva cada imagem decodificada
	var keys []string
	for name, b64 := range images {
		imgBytes, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			continue
		}
		imgPath := filepath.Join(dir, fmt.Sprintf("%d_%s.png", ts, name))
		if err := os.WriteFile(imgPath, imgBytes, 0644); err != nil {
			fmt.Printf("⚠️  [ML] Erro salvando imagem %s: %v\n", name, err)
			continue
		}
		keys = append(keys, name)
	}

	// Salva metadados do sample
	sample := CaptchaSample{
		Type:      captchaType,
		Timestamp: ts,
		Manual:    manual,
		ImageKeys: keys,
	}
	metaPath := filepath.Join(dir, fmt.Sprintf("%d_meta.json", ts))
	metaData, _ := json.MarshalIndent(sample, "", "  ")
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		fmt.Printf("⚠️  [ML] Erro salvando metadados: %v\n", err)
		return
	}

	fmt.Printf("📊 [ML] Sample coletado: %s/%d (%d imagens)\n", captchaType, ts, len(keys))
}
