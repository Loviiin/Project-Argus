

package tiktok

import "errors"

// RawComment representa um comentário com o nick do autor
type RawComment struct {
	Nick string `json:"nick"` // username TikTok do autor
	Text string `json:"text"` // texto do comentário
}

// RawVideoMetadata representa os metadados brutos extraídos de um vídeo do TikTok
type RawVideoMetadata struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	URL         string       `json:"url"`
	Author      string       `json:"author"`
	Comments    []RawComment `json:"comments"`
}

// TikTokAPIResponse representa a resposta da API interna do TikTok para comentários
type TikTokAPIResponse struct {
	Comments []struct {
		Text string `json:"text"`
		User struct {
			UniqueId string `json:"unique_id"`
		} `json:"user"`
	} `json:"comments"`
}

// CaptchaImages contém as URLs das imagens do captcha para envio ao solver
type CaptchaImages struct {
	BackgroundURL string `json:"background_url"` // URL da imagem de fundo
	PieceURL      string `json:"piece_url"`      // URL da peça do quebra-cabeça
}

// CaptchaSolution representa a resposta do solver com a distância a ser arrastada
type CaptchaSolution struct {
	DistanceX float64 `json:"distance_x"` // Distância horizontal em pixels
	Success   bool    `json:"success"`    // Se a solução foi encontrada
	Error     string  `json:"error"`      // Mensagem de erro, se houver
}

// Removido SadCaptcha structs

// CaptchaType representa o tipo de captcha detectado
type CaptchaType int

const (
	CaptchaTypeUnknown CaptchaType = iota
	CaptchaTypeRotate              // Captcha de rotação (alinhar círculos)
	CaptchaTypePuzzle              // Captcha de quebra-cabeça (encaixar peça)
	CaptchaTypeSoftGate            // Captcha overlay que pode ser fechado
)

func (ct CaptchaType) String() string {
	switch ct {
	case CaptchaTypeRotate:
		return "Rotate"
	case CaptchaTypePuzzle:
		return "Puzzle"
	default:
		return "Unknown"
	}
}

var (
	// ErrCaptcha indica que um captcha foi detectado e não resolvido
	ErrCaptcha = errors.New("captcha detectado: necessário resolver")
	// ErrCaptchaTimeout indica que o tempo limite para resolver o captcha foi atingido
	ErrCaptchaTimeout = errors.New("timeout ao aguardar resolução do captcha")
	// ErrCaptchaNotFound indica que os elementos do captcha não foram encontrados
	ErrCaptchaNotFound = errors.New("elementos do captcha não encontrados na página")
	// ErrCaptchaDismissed indica que o CAPTCHA foi descartado (não resolvido, overlay fechado)
	ErrCaptchaDismissed = errors.New("captcha dismissed via close/ESC")
)
