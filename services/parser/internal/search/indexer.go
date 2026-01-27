package search

import (
	"fmt"
	"log"

	"github.com/meilisearch/meilisearch-go"
)

// SearchDoc define a estrutura do documento no Meilisearch
type SearchDoc struct {
	ID         string `json:"id"`
	ServerName string `json:"server_name"`
	InviteCode string `json:"invite_code"`
	SourceURL  string `json:"source_url"`
	Timestamp  int64  `json:"timestamp"`
}

// Indexer é a struct que guarda a conexão aberta
type Indexer struct {
	client    meilisearch.ServiceManager
	indexName string
}

// NewIndexer cria a conexão e garante que o índice existe
func NewIndexer(host, apiKey, indexName string) *Indexer {
	client := meilisearch.New(host, meilisearch.WithAPIKey(apiKey))

	_, err := client.CreateIndex(&meilisearch.IndexConfig{
		Uid:        indexName,
		PrimaryKey: "id",
	})
	if err != nil {
		log.Printf("Aviso Meilisearch: %v", err)
	}

	client.Index(indexName).UpdateSearchableAttributes(&[]string{
		"server_name",
		"invite_code",
		"source_url",
	})

	fmt.Println("Conectado ao Meilisearch!")

	return &Indexer{
		client:    client,
		indexName: indexName,
	}
}

// IndexData recebe o documento pronto e envia
func (i *Indexer) IndexData(doc SearchDoc) error {
	task, err := i.client.Index(i.indexName).AddDocuments([]SearchDoc{doc}, nil)
	if err != nil {
		return fmt.Errorf("erro ao indexar documento: %w", err)
	}

	fmt.Printf("Enviado para Meilisearch (Task UID: %d)\n", task.TaskUID)
	return nil
}
