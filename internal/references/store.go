package references

import (
	"context"
	"strings"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

const (
	projectPrefix   = "PROJECT#"
	referencePrefix = "REF#"
)

// Repositório
type DynamoStore struct {
	table *dynamo.Table
}

// Construção
func NewDynamoStore(t *dynamo.Table) *DynamoStore { return &DynamoStore{table: t} }

// Item
type referenceItem struct {
	PK        string `dynamodbav:"PK"`
	SK        string `dynamodbav:"SK"`
	DOI       string `dynamodbav:"doi"`
	Title     string `dynamodbav:"titulo"`
	Year      int    `dynamodbav:"ano,omitempty"`
	ABNT      string `dynamodbav:"abnt"`
	ABNTHTML  string `dynamodbav:"abntHtml"`
	AddedByID string `dynamodbav:"adicionadaPorId"`
	AddedBy   string `dynamodbav:"adicionadaPor"`
	AddedAt   string `dynamodbav:"adicionadaEm"`
}

// Mapeamento
func (it referenceItem) reference() Reference {
	return Reference{
		ID:        strings.TrimPrefix(it.SK, referencePrefix),
		ProjectID: strings.TrimPrefix(it.PK, projectPrefix),
		DOI:       it.DOI,
		Title:     it.Title,
		Year:      it.Year,
		ABNT:      it.ABNT,
		ABNTHTML:  it.ABNTHTML,
		AddedBy:   it.AddedBy,
		AddedAt:   it.AddedAt,
	}
}

// Gravação
func (s *DynamoStore) PutReference(ctx context.Context, r Reference, addedByID string) error {
	err := dynamo.PutStruct(ctx, s.table, referenceItem{
		PK: projectPrefix + r.ProjectID, SK: referencePrefix + r.ID,
		DOI: r.DOI, Title: r.Title, Year: r.Year, ABNT: r.ABNT, ABNTHTML: r.ABNTHTML,
		AddedByID: addedByID, AddedBy: r.AddedBy, AddedAt: r.AddedAt,
	}, dynamo.Write{Condition: "attribute_not_exists(SK)"})
	if dynamo.ConditionFailed(err) {
		return errDuplicate
	}
	return err
}

// Listagem
func (s *DynamoStore) ReferencesOf(ctx context.Context, projectID string) ([]Reference, error) {
	items, err := dynamo.QueryPartition[referenceItem](ctx, s.table, projectPrefix+projectID, referencePrefix)
	if err != nil {
		return nil, err
	}
	refs := make([]Reference, 0, len(items))
	for _, it := range items {
		refs = append(refs, it.reference())
	}
	return refs, nil
}

// Exclusão
func (s *DynamoStore) DeleteReference(ctx context.Context, projectID, id string) error {
	_, err := s.table.DeleteItem(ctx, dynamo.Key(projectPrefix+projectID, referencePrefix+id), dynamo.Write{Condition: "attribute_exists(SK)"})
	if dynamo.ConditionFailed(err) {
		return errNotFound
	}
	return err
}
