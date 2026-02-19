package sources

// RawComment representa um comentário com o nick do autor
type RawComment struct {
	Nick string `json:"nick"` // username TikTok do autor
	Text string `json:"text"` // texto do comentário
}

type RawVideoMetadata struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	URL         string       `json:"url"`
	Author      string       `json:"author"`
	Comments    []RawComment `json:"comments"`
}

type TikTokAPIResponse struct {
	Comments []struct {
		Text string `json:"text"`
		User struct {
			UniqueId string `json:"unique_id"`
		} `json:"user"`
	} `json:"comments"`
}
