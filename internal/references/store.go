package references

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

// Items (single table), in the project's partition:
//
//	reference  PK=PROJECT#<projectId>  SK=REF#<id derived from the DOI>
const (
	projectPrefix   = "PROJECT#"
	referencePrefix = "REF#"
)

type DynamoStore struct {
	table *dynamo.Table
}

func NewDynamoStore(t *dynamo.Table) *DynamoStore { return &DynamoStore{table: t} }

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

func (s *DynamoStore) PutReference(ctx context.Context, r Reference, addedByID string) error {
	item, err := attributevalue.MarshalMap(referenceItem{
		PK: projectPrefix + r.ProjectID, SK: referencePrefix + r.ID,
		DOI: r.DOI, Title: r.Title, Year: r.Year, ABNT: r.ABNT, ABNTHTML: r.ABNTHTML,
		AddedByID: addedByID, AddedBy: r.AddedBy, AddedAt: r.AddedAt,
	})
	if err != nil {
		return err
	}
	_, err = s.table.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.table.Name),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(SK)"),
	})
	if dynamo.ConditionFailed(err) {
		return errDuplicate
	}
	return err
}

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

func (s *DynamoStore) DeleteReference(ctx context.Context, projectID, id string) error {
	_, err := s.table.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(s.table.Name),
		Key:                 dynamo.Key(projectPrefix+projectID, referencePrefix+id),
		ConditionExpression: aws.String("attribute_exists(SK)"),
	})
	if dynamo.ConditionFailed(err) {
		return errNotFound
	}
	return err
}
