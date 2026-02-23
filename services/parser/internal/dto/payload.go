package dto

type OcrMessage struct {
	SourcePath  string                 `json:"source_path"`
	TextContent string                 `json:"text_content"`
	AuthorID    string                 `json:"author_id,omitempty"`
	SourceType  string                 `json:"source_type,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// DiscordEnrichJob é o payload para o tópico jobs.enrich.discord
type DiscordEnrichJob struct {
	InviteCode string `json:"invite_code"`
}
