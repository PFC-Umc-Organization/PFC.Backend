package materials

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

// Items (single table):
//
//	material  PK=MATERIAL#<id>  SK=PROFILE  GSI1PK=MATERIAIS  GSI1SK=MATERIAL#<id>
const (
	materialPrefix = "MATERIAL#"
	partition      = "MATERIAIS"
	profileSK      = "PROFILE"
)

type DynamoStore struct {
	table *dynamo.Table
}

func NewDynamoStore(t *dynamo.Table) *DynamoStore { return &DynamoStore{table: t} }

type item struct {
	PK          string  `dynamodbav:"PK"`
	SK          string  `dynamodbav:"SK"`
	GSI1PK      string  `dynamodbav:"GSI1PK"`
	GSI1SK      string  `dynamodbav:"GSI1SK"`
	Title       string  `dynamodbav:"titulo"`
	Description string  `dynamodbav:"descricao"`
	Kind        Kind    `dynamodbav:"tipo"`
	URL         string  `dynamodbav:"url,omitempty"`
	FileKey     string  `dynamodbav:"arquivoChave,omitempty"`
	File        string  `dynamodbav:"arquivo,omitempty"`
	CohortID    *string `dynamodbav:"turmaId,omitempty"`
	PublishedAt string  `dynamodbav:"publicadoEm"`
	PublishedBy string  `dynamodbav:"publicadoPor"`
	AuthorSub   string  `dynamodbav:"autorSub"`
}

func (it item) material() Material {
	return Material{
		ID:          strings.TrimPrefix(it.PK, materialPrefix),
		Title:       it.Title,
		Description: it.Description,
		Kind:        it.Kind,
		URL:         it.URL,
		File:        it.File,
		CohortID:    it.CohortID,
		PublishedAt: it.PublishedAt,
		PublishedBy: it.PublishedBy,
		fileKey:     it.FileKey,
		AuthorID:    it.AuthorSub,
	}
}

func key(id string) dynamo.Item { return dynamo.Key(materialPrefix+id, profileSK) }

func (s *DynamoStore) Create(ctx context.Context, m Material) error {
	err := dynamo.PutStruct(ctx, s.table, item{
		PK: materialPrefix + m.ID, SK: profileSK, GSI1PK: partition, GSI1SK: materialPrefix + m.ID,
		Title: m.Title, Description: m.Description, Kind: m.Kind, URL: m.URL, FileKey: m.fileKey, File: m.File,
		CohortID: m.CohortID, PublishedAt: m.PublishedAt, PublishedBy: m.PublishedBy, AuthorSub: m.AuthorID,
	}, dynamo.Write{Condition: "attribute_not_exists(PK)"})
	if dynamo.ConditionFailed(err) {
		return errDuplicate
	}
	return err
}

func (s *DynamoStore) Get(ctx context.Context, id string) (Material, bool, error) {
	it, found, err := dynamo.Get[item](ctx, s.table, key(id))
	return it.material(), found, err
}

func (s *DynamoStore) List(ctx context.Context) ([]Material, error) {
	items, err := dynamo.QueryIndex[item](ctx, s.table, partition)
	if err != nil {
		return nil, err
	}
	out := make([]Material, 0, len(items))
	for _, it := range items {
		out = append(out, it.material())
	}
	return out, nil
}

func (s *DynamoStore) Delete(ctx context.Context, id string) (Material, error) {
	old, err := s.table.DeleteItem(ctx, key(id), dynamo.Write{Condition: "attribute_exists(PK)", Return: types.ReturnValueAllOld})
	if dynamo.ConditionFailed(err) {
		return Material{}, errNotFound
	}
	if err != nil {
		return Material{}, err
	}
	var it item
	if err := attributevalue.UnmarshalMap(old, &it); err != nil {
		return Material{}, err
	}
	return it.material(), nil
}
