package dto

type OcrMessage struct {
	SourcePath  string `json:"source_path"`
	TextContent string `json:"text_content"`
}
