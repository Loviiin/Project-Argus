//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"discovery/internal/sources/tiktok"
)

func main() {
	// Exemplo 1: Usando o source diretamente (recomendado para novo código)
	fmt.Println("=== Exemplo de uso do novo pacote tiktok ===\n")

	source := tiktok.NewSource()

	// Buscar por tag
	fmt.Println("1. Buscando vídeos pela tag 'viral'...")
	results, err := source.Fetch("viral")
	if err != nil {
		log.Printf("Erro buscando tag: %v\n", err)
	} else {
		fmt.Printf("✓ Encontrados %d vídeos\n\n", len(results))
		for i, video := range results {
			fmt.Printf("Vídeo %d:\n", i+1)
			fmt.Printf("  ID: %s\n", video.ID)
			fmt.Printf("  URL: %s\n", video.URL)
			fmt.Printf("  Descrição: %s\n", video.Description)
			fmt.Printf("  Comentários: %d\n\n", len(video.Comments))
		}
	}

	// Buscar por URL direta
	fmt.Println("2. Buscando vídeo por URL direta...")
	videoURL := "https://www.tiktok.com/@user/video/123456789"
	results, err = source.Fetch(videoURL)
	if err != nil {
		log.Printf("Erro buscando URL: %v\n", err)
	} else {
		fmt.Printf("✓ Vídeo processado com sucesso\n\n")
	}

	// Exemplo 2: Usando componentes individuais
	fmt.Println("\n=== Exemplo de uso dos componentes individuais ===\n")

	// Exemplo de CaptchaSolver (ainda em mock)
	solver, err := tiktok.NewCaptchaSolver()
	if err != nil {
		log.Fatalf("Erro criando solver: %v", err)
	}
	defer solver.Close()

	// Simula uma requisição de captcha
	images := &tiktok.CaptchaImages{
		BackgroundURL: "https://example.com/captcha_bg.png",
		PieceURL:      "https://example.com/captcha_piece.png",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	solution, err := solver.RequestSolution(ctx, images)
	if err != nil {
		log.Printf("Erro obtendo solução: %v\n", err)
	} else {
		fmt.Printf("✓ Solução recebida: distância = %.2f pixels\n", solution.DistanceX)
	}

	// Exemplo de uso do movimento humanizado
	// (Requer uma página Rod ativa, este é apenas ilustrativo)
	/*
		page := ... // sua página Rod
		slider := ... // elemento do slider

		// Arrasta o slider 150 pixels para a direita
		err = tiktok.DragSlider(page, slider, 150.0)
		if err != nil {
			log.Printf("Erro arrastando slider: %v", err)
		}
	*/

	fmt.Println("\n=== Exemplos concluídos ===")
}
