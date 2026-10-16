package dynamo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type Item = map[string]types.AttributeValue

// Table is the single table plus its overloaded GSI (GSI1PK/GSI1SK).
type Table struct {
	Client *dynamodb.Client
	Name   string
	Index  string
}

func S(v string) types.AttributeValue { return &types.AttributeValueMemberS{Value: v} }

func N(v int) types.AttributeValue { return &types.AttributeValueMemberN{Value: fmt.Sprint(v)} }

func Key(pk, sk string) Item { return Item{"PK": S(pk), "SK": S(sk)} }

func String(it Item, attr string) string {
	if v, ok := it[attr].(*types.AttributeValueMemberS); ok {
		return v.Value
	}
	return ""
}

// QueryAll follows LastEvaluatedKey: a single Query stops at 1 MB without
// reporting it.
func (t *Table) QueryAll(ctx context.Context, in *dynamodb.QueryInput) ([]Item, error) {
	in.TableName = aws.String(t.Name)
	var items []Item
	for {
		out, err := t.Client.Query(ctx, in)
		if err != nil {
			return nil, err
		}
		items = append(items, out.Items...)
		if len(out.LastEvaluatedKey) == 0 {
			return items, nil
		}
		in.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

// Query never returns nil, so empty results encode as [] instead of null.
func Query[T any](ctx context.Context, t *Table, in *dynamodb.QueryInput) ([]T, error) {
	items, err := t.QueryAll(ctx, in)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(items))
	if err := attributevalue.UnmarshalListOfMaps(items, &out); err != nil {
		return nil, fmt.Errorf("unexpected item shape: %w", err)
	}
	return out, nil
}

func QueryIndex[T any](ctx context.Context, t *Table, gsi1pk string) ([]T, error) {
	return Query[T](ctx, t, &dynamodb.QueryInput{
		IndexName:                 aws.String(t.Index),
		KeyConditionExpression:    aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: Item{":pk": S(gsi1pk)},
	})
}

func QueryPartition[T any](ctx context.Context, t *Table, pk, skPrefix string) ([]T, error) {
	return Query[T](ctx, t, &dynamodb.QueryInput{
		KeyConditionExpression:    aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: Item{":pk": S(pk), ":sk": S(skPrefix)},
	})
}

func Get[T any](ctx context.Context, t *Table, key Item, projection ...string) (v T, found bool, err error) {
	in := &dynamodb.GetItemInput{TableName: aws.String(t.Name), Key: key}
	if len(projection) > 0 {
		in.ProjectionExpression = aws.String(strings.Join(projection, ", "))
	}
	out, err := t.Client.GetItem(ctx, in)
	if err != nil || out.Item == nil {
		return v, false, err
	}
	if err := attributevalue.UnmarshalMap(out.Item, &v); err != nil {
		return v, false, fmt.Errorf("unexpected item shape: %w", err)
	}
	return v, true, nil
}

func (t *Table) Exists(ctx context.Context, key Item) (bool, error) {
	_, found, err := Get[struct{}](ctx, t, key, "PK")
	return found, err
}

func ConditionFailed(err error) bool {
	var c *types.ConditionalCheckFailedException
	return errors.As(err, &c)
}
