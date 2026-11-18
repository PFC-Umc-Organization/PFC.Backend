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

// ---------------------------------------------------------------------------
// Tabela
// ---------------------------------------------------------------------------

// Item
type Item = map[string]types.AttributeValue

// Cliente
type Table struct {
	Client *dynamodb.Client
	Name   string
	Index  string
}

// Conversão
func S(v string) types.AttributeValue { return &types.AttributeValueMemberS{Value: v} }

// Conversão
func N(v int) types.AttributeValue { return &types.AttributeValueMemberN{Value: fmt.Sprint(v)} }

// Chave
func Key(pk, sk string) Item { return Item{"PK": S(pk), "SK": S(sk)} }

// Texto
func String(it Item, attr string) string {
	if v, ok := it[attr].(*types.AttributeValueMemberS); ok {
		return v.Value
	}
	return ""
}

// ---------------------------------------------------------------------------
// Consultas
// ---------------------------------------------------------------------------

// Consulta
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

// Consulta
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

// Consulta
func QueryIndex[T any](ctx context.Context, t *Table, gsi1pk string) ([]T, error) {
	return Query[T](ctx, t, &dynamodb.QueryInput{
		IndexName:                 aws.String(t.Index),
		KeyConditionExpression:    aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: Item{":pk": S(gsi1pk)},
	})
}

// Consulta
func QueryPartition[T any](ctx context.Context, t *Table, pk, skPrefix string) ([]T, error) {
	return Query[T](ctx, t, &dynamodb.QueryInput{
		KeyConditionExpression:    aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: Item{":pk": S(pk), ":sk": S(skPrefix)},
	})
}

// Consulta
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

// Verificação
func (t *Table) Exists(ctx context.Context, key Item) (bool, error) {
	_, found, err := Get[struct{}](ctx, t, key, "PK")
	return found, err
}

// ---------------------------------------------------------------------------
// Erros
// ---------------------------------------------------------------------------

var ErrConditionFailed = errors.New("dynamo: condition failed")

// Erro
type ConditionFailedError struct {
	Old   Item
	cause error
}

// Mensagem
func (e *ConditionFailedError) Error() string { return ErrConditionFailed.Error() }

// Comparação
func (e *ConditionFailedError) Is(target error) bool { return target == ErrConditionFailed }

// Desembrulho
func (e *ConditionFailedError) Unwrap() error { return e.cause }

// ---------------------------------------------------------------------------
// Escrita
// ---------------------------------------------------------------------------

// Escrita
type Write struct {
	Condition string
	Values    Item
	Return    types.ReturnValue
	OldOnFail bool
}

// Retorno
func (w Write) failureReturn() types.ReturnValuesOnConditionCheckFailure {
	if w.OldOnFail {
		return types.ReturnValuesOnConditionCheckFailureAllOld
	}
	return ""
}

// Tradução
func conditionError(err error) error {
	var c *types.ConditionalCheckFailedException
	if errors.As(err, &c) {
		return &ConditionFailedError{Old: c.Item, cause: err}
	}
	return err
}

// Gravação
func (t *Table) PutItem(ctx context.Context, item Item, w Write) error {
	_, err := t.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:                           aws.String(t.Name),
		Item:                                item,
		ConditionExpression:                 optional(w.Condition),
		ExpressionAttributeValues:           optionalValues(w.Values),
		ReturnValuesOnConditionCheckFailure: w.failureReturn(),
	})
	return conditionError(err)
}

// Atualização
func (t *Table) UpdateItem(ctx context.Context, key Item, expression string, w Write) (Item, error) {
	out, err := t.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                           aws.String(t.Name),
		Key:                                 key,
		UpdateExpression:                    aws.String(expression),
		ConditionExpression:                 optional(w.Condition),
		ExpressionAttributeValues:           optionalValues(w.Values),
		ReturnValues:                        w.Return,
		ReturnValuesOnConditionCheckFailure: w.failureReturn(),
	})
	if err != nil {
		return nil, conditionError(err)
	}
	return out.Attributes, nil
}

// Exclusão
func (t *Table) DeleteItem(ctx context.Context, key Item, w Write) (Item, error) {
	out, err := t.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:                           aws.String(t.Name),
		Key:                                 key,
		ConditionExpression:                 optional(w.Condition),
		ExpressionAttributeValues:           optionalValues(w.Values),
		ReturnValues:                        w.Return,
		ReturnValuesOnConditionCheckFailure: w.failureReturn(),
	})
	if err != nil {
		return nil, conditionError(err)
	}
	return out.Attributes, nil
}

// Gravação
func PutStruct(ctx context.Context, t *Table, v any, w Write) error {
	item, err := attributevalue.MarshalMap(v)
	if err != nil {
		return err
	}
	return t.PutItem(ctx, item, w)
}

// Verificação
func ConditionFailed(err error) bool {
	var c *types.ConditionalCheckFailedException
	return errors.Is(err, ErrConditionFailed) || errors.As(err, &c)
}
