package sources

type RawVideoMetadata struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	URL         string   `json:"url"`
	Author      string   `json:"author"`
	Comments    []string `json:"comments"`
}

type TikTokAPIResponse struct {
	Comments []struct {
		Text string `json:"text"`
		User struct {
			UniqueId string `json:"unique_id"`
		} `json:"user"`
	} `json:"comments"`
}
