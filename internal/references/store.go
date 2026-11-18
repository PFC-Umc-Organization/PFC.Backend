package references

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

// ---------------------------------------------------------------------------
// Modelo
// ---------------------------------------------------------------------------

// Itens
// Items (single table), in the project's partition:
//
//	reference  PK=PROJECT#<projectId>  SK=REF#<id derived from the DOI>
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

// Conversão
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

// Conversão
func itemOf(r Reference, addedByID string) referenceItem {
	return referenceItem{
		PK: projectPrefix + r.ProjectID, SK: referencePrefix + r.ID,
		DOI: r.DOI, Title: r.Title, Year: r.Year, ABNT: r.ABNT, ABNTHTML: r.ABNTHTML,
		AddedByID: addedByID, AddedBy: r.AddedBy, AddedAt: r.AddedAt,
	}
}

// ---------------------------------------------------------------------------
// Persistência
// ---------------------------------------------------------------------------

// Criação
func (s *DynamoStore) PutReference(ctx context.Context, r Reference, addedByID string) error {
	err := dynamo.PutStruct(ctx, s.table, itemOf(r, addedByID), dynamo.Write{Condition: "attribute_not_exists(SK)"})
	if dynamo.ConditionFailed(err) {
		return errDuplicate
	}
	return err
}

// Substituição
// ReplaceReference keeps the item key derived from the DOI: the same DOI
// overwrites the item in place; another DOI deletes the old item and puts
// the new one in one transaction, so a failure never loses the reference.
func (s *DynamoStore) ReplaceReference(ctx context.Context, oldID string, r Reference, addedByID string) error {
	if oldID == r.ID {
		err := dynamo.PutStruct(ctx, s.table, itemOf(r, addedByID), dynamo.Write{Condition: "attribute_exists(SK)"})
		if dynamo.ConditionFailed(err) {
			return errNotFound
		}
		return err
	}
	item, err := attributevalue.MarshalMap(itemOf(r, addedByID))
	if err != nil {
		return err
	}
	err = s.table.Transact(ctx,
		s.table.Delete(dynamo.Key(projectPrefix+r.ProjectID, referencePrefix+oldID), "attribute_exists(SK)", nil),
		s.table.Put(item, "attribute_not_exists(SK)", nil),
	)
	switch {
	case dynamo.FailedAt(err, 0):
		return errNotFound
	case dynamo.FailedAt(err, 1):
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

// Remoção
func (s *DynamoStore) DeleteReference(ctx context.Context, projectID, id string) error {
	_, err := s.table.DeleteItem(ctx, dynamo.Key(projectPrefix+projectID, referencePrefix+id), dynamo.Write{Condition: "attribute_exists(SK)"})
	if dynamo.ConditionFailed(err) {
		return errNotFound
	}
	return err
}
