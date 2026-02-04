package tiktok

type TikTokRodSource struct {
	source *Source
}

func NewTikTokRodSource() *TikTokRodSource {
	return &TikTokRodSource{
		source: NewSource(),
	}
}

func (t *TikTokRodSource) Name() string {
	return t.source.Name()
}

func (t *TikTokRodSource) Fetch(query string) ([]RawVideoMetadata, error) {
	return t.source.Fetch(query)
}
