package announcements

import (
	"context"
	"strings"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

const (
	announcementPrefix = "AVISO#"
	partition          = "AVISOS"
	profileSK          = "PROFILE"
)

// Repositório
type DynamoStore struct {
	table *dynamo.Table
}

// Construção
func NewDynamoStore(t *dynamo.Table) *DynamoStore { return &DynamoStore{table: t} }

// Item
type item struct {
	PK         string `dynamodbav:"PK"`
	SK         string `dynamodbav:"SK"`
	GSI1PK     string `dynamodbav:"GSI1PK"`
	GSI1SK     string `dynamodbav:"GSI1SK"`
	Title      string `dynamodbav:"titulo"`
	Content    string `dynamodbav:"conteudo"`
	CohortID   string `dynamodbav:"turmaId,omitempty"`
	AuthorID   string `dynamodbav:"autorId"`
	AuthorName string `dynamodbav:"autorNome"`
	AuthorRole string `dynamodbav:"autorPapel"`
	CreatedAt  string `dynamodbav:"criadoEm"`
}

// Mapeamento
func (it item) announcement() Announcement {
	return Announcement{
		ID: strings.TrimPrefix(it.PK, announcementPrefix), Title: it.Title, Content: it.Content,
		CohortID: it.CohortID, AuthorID: it.AuthorID, AuthorName: it.AuthorName,
		AuthorRole: it.AuthorRole, CreatedAt: it.CreatedAt,
	}
}

// Chave
func key(id string) dynamo.Item { return dynamo.Key(announcementPrefix+id, profileSK) }

// Criação
func (s *DynamoStore) Create(ctx context.Context, a Announcement) error {
	err := dynamo.PutStruct(ctx, s.table, item{
		PK: announcementPrefix + a.ID, SK: profileSK, GSI1PK: partition, GSI1SK: announcementPrefix + a.ID,
		Title: a.Title, Content: a.Content, CohortID: a.CohortID, AuthorID: a.AuthorID,
		AuthorName: a.AuthorName, AuthorRole: a.AuthorRole, CreatedAt: a.CreatedAt,
	}, dynamo.Write{Condition: "attribute_not_exists(PK)"})
	if dynamo.ConditionFailed(err) {
		return errDuplicate
	}
	return err
}

// Consulta
func (s *DynamoStore) Get(ctx context.Context, id string) (Announcement, bool, error) {
	it, found, err := dynamo.Get[item](ctx, s.table, key(id))
	return it.announcement(), found, err
}

// Listagem
func (s *DynamoStore) List(ctx context.Context) ([]Announcement, error) {
	items, err := dynamo.QueryIndex[item](ctx, s.table, partition)
	if err != nil {
		return nil, err
	}
	out := make([]Announcement, 0, len(items))
	for _, it := range items {
		out = append(out, it.announcement())
	}
	return out, nil
}

// Exclusão
func (s *DynamoStore) Delete(ctx context.Context, id string) error {
	_, err := s.table.DeleteItem(ctx, key(id), dynamo.Write{Condition: "attribute_exists(PK)"})
	if dynamo.ConditionFailed(err) {
		return errNotFound
	}
	return err
}
