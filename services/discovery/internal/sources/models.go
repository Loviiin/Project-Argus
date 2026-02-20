package sources

// DiscoveredVideo contém apenas o ID e URL de um vídeo descoberto (pós-filtro Redis).
// É o payload mínimo que o Discovery publica no NATS.
type DiscoveredVideo struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// RawComment representa um comentário com o nick do autor.
// Usado pelo Scraper Worker ao processar o vídeo.
type RawComment struct {
	Nick string `json:"nick"` // username TikTok do autor
	Text string `json:"text"` // texto do comentário
}

// RawVideoMetadata representa os metadados completos extraídos de um vídeo do TikTok.
// Usado pelo Scraper Worker ao processar o vídeo.
type RawVideoMetadata struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	URL         string       `json:"url"`
	Author      string       `json:"author"`
	Comments    []RawComment `json:"comments"`
}

// TikTokAPIResponse representa a resposta da API interna do TikTok para comentários.
type TikTokAPIResponse struct {
	Comments []struct {
		Text string `json:"text"`
		User struct {
			UniqueId string `json:"unique_id"`
		} `json:"user"`
	} `json:"comments"`
}
