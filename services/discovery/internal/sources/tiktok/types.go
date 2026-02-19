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

// SadCaptchaRotateRequest é o JSON enviado para a API do SadCaptcha (Rotate)
type SadCaptchaRotateRequest struct {
	OuterImageB64 string `json:"outerImageB64"` // Imagem externa em Base64
	InnerImageB64 string `json:"innerImageB64"` // Imagem interna em Base64
}

// SadCaptchaPuzzleRequest é o JSON enviado para a API do SadCaptcha (Puzzle)
type SadCaptchaPuzzleRequest struct {
	PuzzleImageB64 string `json:"puzzleImageB64"` // Imagem do puzzle em Base64
	PieceImageB64  string `json:"pieceImageB64"`  // Imagem da peça em Base64
}

// SadCaptchaRotateResponse representa a resposta da API para captcha Rotate
type SadCaptchaRotateResponse struct {
	Angle   float64 `json:"angle"`             // Ângulo da solução (0 a 360)
	ErrorID int     `json:"errorId,omitempty"` // ID do erro, se houver
	Message string  `json:"message,omitempty"` // Mensagem de erro
}

// SadCaptchaPuzzleResponse representa a resposta da API para captcha Puzzle
type SadCaptchaPuzzleResponse struct {
	Slide   float64 `json:"slide"`             // Distância em pixels para arrastar
	ErrorID int     `json:"errorId,omitempty"` // ID do erro, se houver
	Message string  `json:"message,omitempty"` // Mensagem de erro
}

// CaptchaType representa o tipo de captcha detectado
type CaptchaType int

const (
	CaptchaTypeUnknown CaptchaType = iota
	CaptchaTypeRotate              // Captcha de rotação (alinhar círculos)
	CaptchaTypePuzzle              // Captcha de quebra-cabeça (encaixar peça)
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
	// ErrSadCaptchaAPIKey indica que a API key do SadCaptcha não foi configurada
	ErrSadCaptchaAPIKey = errors.New("SADCAPTCHA_API_KEY não configurada")
	// ErrSadCaptchaFailed indica que a API do SadCaptcha retornou erro
	ErrSadCaptchaFailed = errors.New("API do SadCaptcha falhou")
)
