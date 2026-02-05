package tiktok

// TikTokRodSource é um wrapper para manter compatibilidade com o código existente
// A implementação real agora está no pacote tiktok/
type TikTokRodSource struct {
	source *Source
}

// NewTikTokRodSource cria uma nova instância do scraper TikTok
func NewTikTokRodSource() *TikTokRodSource {
	return &TikTokRodSource{
		source: NewSource(),
	}
}

// Name retorna o nome identificador do source
func (t *TikTokRodSource) Name() string {
	return t.source.Name()
}

// Fetch executa a busca e extração de dados do TikTok
func (t *TikTokRodSource) Fetch(query string) ([]RawVideoMetadata, error) {
	results, err := t.source.Fetch(query)
	if err != nil {
		return nil, err
	}

	// Results já são do mesmo tipo, apenas retorna
	return results, nil
}
