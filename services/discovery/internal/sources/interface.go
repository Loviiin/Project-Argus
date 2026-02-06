package sources

// Source é a interface que qualquer fonte de dados (TikTok, YouTube) deve implementar.
type Source interface {
	Name() string
	// Fetch busca dados baseados em um termo ou URL e retorna uma lista de metadados.
	Fetch(query string) ([]RawVideoMetadata, error)
}
