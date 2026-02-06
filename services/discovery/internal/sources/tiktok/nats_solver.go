package tiktok

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// CaptchaSolver é responsável por comunicar com o serviço de resolução de captcha via NATS
type CaptchaSolver struct {
	// TODO: Adicionar cliente NATS quando integrado
	// nc *nats.Conn
	// js jetstream.JetStream
}

// NewCaptchaSolver cria uma nova instância do solver
func NewCaptchaSolver() (*CaptchaSolver, error) {
	// TODO: Conectar ao NATS
	// nc, err := nats.Connect(natsURL)
	// if err != nil {
	// 	return nil, fmt.Errorf("erro conectando ao NATS: %w", err)
	// }
	//
	// js, err := jetstream.New(nc)
	// if err != nil {
	// 	return nil, fmt.Errorf("erro criando JetStream: %w", err)
	// }

	return &CaptchaSolver{
		// nc: nc,
		// js: js,
	}, nil
}

// RequestSolution envia as imagens do captcha para o tópico NATS e aguarda a resposta
// Retorna a distância X que o slider deve ser arrastado
func (s *CaptchaSolver) RequestSolution(ctx context.Context, images *CaptchaImages) (*CaptchaSolution, error) {
	fmt.Println("[NATS] Preparando requisição de solução de captcha...")

	// TODO: Implementar envio real via NATS
	// Estrutura esperada:
	// 1. Publicar no tópico: jobs.captcha.solve
	// 2. Payload: JSON com BackgroundURL e PieceURL
	// 3. Aguardar resposta no tópico: jobs.captcha.result.<request_id>
	// 4. Parse da resposta com DistanceX

	/*
		// Exemplo de implementação futura:

		payload, err := json.Marshal(images)
		if err != nil {
			return nil, fmt.Errorf("erro serializando payload: %w", err)
		}

		requestID := generateRequestID()
		replySubject := fmt.Sprintf("jobs.captcha.result.%s", requestID)

		// Cria subscriber para a resposta
		sub, err := s.nc.SubscribeSync(replySubject)
		if err != nil {
			return nil, fmt.Errorf("erro criando subscriber: %w", err)
		}
		defer sub.Unsubscribe()

		// Publica a requisição
		err = s.nc.PublishRequest("jobs.captcha.solve", replySubject, payload)
		if err != nil {
			return nil, fmt.Errorf("erro publicando requisição: %w", err)
		}

		fmt.Printf("[NATS] Requisição enviada. Aguardando resposta em %s...\n", replySubject)

		// Aguarda resposta com timeout
		msg, err := sub.NextMsgWithContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("timeout aguardando resposta do solver: %w", err)
		}

		var solution CaptchaSolution
		if err := json.Unmarshal(msg.Data, &solution); err != nil {
			return nil, fmt.Errorf("erro parseando resposta: %w", err)
		}

		if !solution.Success {
			return nil, fmt.Errorf("solver falhou: %s", solution.Error)
		}

		fmt.Printf("[NATS] Solução recebida: distância = %.2f pixels\n", solution.DistanceX)
		return &solution, nil
	*/

	// MOCK: Por enquanto, simula uma resposta após um delay
	fmt.Println("[NATS] MOCK: Simulando envio para NATS e resposta do Vision service...")
	fmt.Printf("[NATS] MOCK: Imagens - Background: %s\n", images.BackgroundURL)
	fmt.Printf("[NATS] MOCK: Imagens - Piece: %s\n", images.PieceURL)

	select {
	case <-time.After(2 * time.Second):
		// Simula processamento
		mockSolution := &CaptchaSolution{
			DistanceX: 150.0 + float64(time.Now().Unix()%50), // Varia um pouco para parecer real
			Success:   true,
			Error:     "",
		}
		fmt.Printf("[NATS] MOCK: Resposta recebida com distância = %.2f pixels\n", mockSolution.DistanceX)
		return mockSolution, nil

	case <-ctx.Done():
		return nil, fmt.Errorf("contexto cancelado: %w", ctx.Err())
	}
}

// Close fecha a conexão com o NATS
func (s *CaptchaSolver) Close() {
	// TODO: Fechar conexão quando implementado
	// if s.nc != nil {
	// 	s.nc.Close()
	// }
}

// generateRequestID gera um ID único para a requisição
func generateRequestID() string {
	// TODO: Implementar geração de ID único
	// Pode usar UUID, timestamp + random, etc.
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}

// --- Funções auxiliares para integração futura ---

// PublishCaptchaJob publica um job de captcha no NATS (stub para referência futura)
func PublishCaptchaJob(images *CaptchaImages) error {
	payload, err := json.Marshal(images)
	if err != nil {
		return err
	}

	fmt.Printf("[NATS] TODO: Publicar no tópico 'jobs.captcha.solve': %s\n", string(payload))
	return nil
}

// SubscribeCaptchaResults assina o tópico de resultados (stub para referência futura)
func SubscribeCaptchaResults(requestID string) (*CaptchaSolution, error) {
	topic := fmt.Sprintf("jobs.captcha.result.%s", requestID)
	fmt.Printf("[NATS] TODO: Assinar tópico: %s\n", topic)

	// Aqui entraria a lógica de subscriber do NATS
	return nil, fmt.Errorf("não implementado")
}

/*
Guia de Integração NATS:

1. Adicionar dependências no go.mod:
   - github.com/nats-io/nats.go
   - github.com/nats-io/nats.go/jetstream

2. Configuração:
   - URL do NATS (ex: nats://localhost:4222)
   - Credenciais se necessário

3. Tópicos:
   - Publicação: "jobs.captcha.solve"
   - Resposta: "jobs.captcha.result.<request_id>"

4. Payload de Request (JSON):
   {
     "background_url": "data:image/png;base64,...",
     "piece_url": "data:image/png;base64,...",
     "request_id": "req_123456789"
   }

5. Payload de Response (JSON):
   {
     "distance_x": 150.5,
     "success": true,
     "error": ""
   }

6. Timeout:
   - Sugestão: 30 segundos para processamento de imagem

7. Retry:
   - Implementar retry com backoff exponencial
   - Máximo 3 tentativas
*/
