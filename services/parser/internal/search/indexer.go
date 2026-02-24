package search

import (
	"fmt"
	"log"

	"github.com/meilisearch/meilisearch-go"
)

// SearchDoc define a estrutura do documento no Meilisearch
type SearchDoc struct {
	InviteCode         string `json:"invite_code"`
	ServerName         string `json:"server_name"`
	SourceURL          string `json:"source_url"`
	TimestampFormatted string `json:"timestamp_formatted,omitempty"`
	MemberCount        int    `json:"member_count"`
	Icon               string `json:"icon,omitempty"`
	Status             string `json:"status,omitempty"`
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
		PrimaryKey: "invite_code", // Alterado de "id" para "invite_code" para garantir o Upsert e não duplicar dados
	})
	if err != nil {
		log.Printf("Aviso Meilisearch: %v", err)
	}

	client.Index(indexName).UpdateSearchableAttributes(&[]string{
		"server_name",
		"invite_code",
		"source_url",
		"status",
	})

	client.Index(indexName).UpdateSortableAttributes(&[]string{
		"member_count",
	})

	filterableAttrs := []interface{}{"member_count", "status"}
	client.Index(indexName).UpdateFilterableAttributes(&filterableAttrs)

	fmt.Println("Conectado ao Meilisearch!")

	return &Indexer{
		client:    client,
		indexName: indexName,
	}
}

// IndexData recebe o documento pronto e atualiza o index parcialmente
func (i *Indexer) IndexData(doc map[string]interface{}) error {
	pk := "invite_code"
	task, err := i.client.Index(i.indexName).UpdateDocuments([]map[string]interface{}{doc}, &meilisearch.DocumentOptions{PrimaryKey: &pk})
	if err != nil {
		return fmt.Errorf("erro ao indexar documento: %w", err)
	}

	fmt.Printf("Enviado para Meilisearch (Task UID: %d) via Upsert Parcial (PK: %s)\n", task.TaskUID, doc["invite_code"])
	return nil
}

// UpdateData realiza um Partial Update. Usamos map[string]interface{} para evitar
// que zero-values (strings vazias, 0) de uma struct sobrescrevam os dados originais no banco.
func (i *Indexer) UpdateData(doc map[string]interface{}) error {
	pk := "invite_code"
	task, err := i.client.Index(i.indexName).UpdateDocuments([]map[string]interface{}{doc}, &meilisearch.DocumentOptions{PrimaryKey: &pk})
	if err != nil {
		return fmt.Errorf("erro ao atualizar documento: %w", err)
	}

	fmt.Printf("Atualizado no Meilisearch (Task UID: %d) via Partial Update (PK: %v)\n", task.TaskUID, doc["invite_code"])
	return nil
}

// GetDocument busca um documento específico no Meilisearch
func (i *Indexer) GetDocument(pk string) (*SearchDoc, error) {
	var doc SearchDoc
	err := i.client.Index(i.indexName).GetDocument(pk, &meilisearch.DocumentQuery{}, &doc)
	if err != nil {
		return nil, err
	}
	return &doc, nil
}
