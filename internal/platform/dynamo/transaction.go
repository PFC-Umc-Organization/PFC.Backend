package dynamo

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// ConditionError lists the positions of the transaction items whose
// condition was rejected.
type ConditionError struct {
	Failed []int
	cause  error
}

func (e *ConditionError) Error() string {
	return fmt.Sprintf("condition rejected for transaction items %v", e.Failed)
}

func (e *ConditionError) Unwrap() error { return e.cause }

func FailedAt(err error, i int) bool {
	var c *ConditionError
	return errors.As(err, &c) && slices.Contains(c.Failed, i)
}

func (t *Table) Transact(ctx context.Context, items ...types.TransactWriteItem) error {
	_, err := t.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	var canceled *types.TransactionCanceledException
	if !errors.As(err, &canceled) {
		return err
	}
	var failed []int
	for i, reason := range canceled.CancellationReasons {
		if aws.ToString(reason.Code) == "ConditionalCheckFailed" {
			failed = append(failed, i)
		}
	}
	if len(failed) == 0 {
		return err
	}
	return &ConditionError{Failed: failed, cause: err}
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return aws.String(s)
}

func optionalValues(v Item) Item {
	if len(v) == 0 {
		return nil
	}
	return v
}

func (t *Table) Put(item Item, condition string, values Item) types.TransactWriteItem {
	return types.TransactWriteItem{Put: &types.Put{
		TableName:                 aws.String(t.Name),
		Item:                      item,
		ConditionExpression:       optional(condition),
		ExpressionAttributeValues: optionalValues(values),
	}}
}

func (t *Table) Delete(key Item, condition string, values Item) types.TransactWriteItem {
	return types.TransactWriteItem{Delete: &types.Delete{
		TableName:                 aws.String(t.Name),
		Key:                       key,
		ConditionExpression:       optional(condition),
		ExpressionAttributeValues: optionalValues(values),
	}}
}

func (t *Table) Update(key Item, expression, condition string, values Item) types.TransactWriteItem {
	return types.TransactWriteItem{Update: &types.Update{
		TableName:                 aws.String(t.Name),
		Key:                       key,
		UpdateExpression:          aws.String(expression),
		ConditionExpression:       optional(condition),
		ExpressionAttributeValues: optionalValues(values),
	}}
}

func (t *Table) Check(key Item, condition string, values Item) types.TransactWriteItem {
	return types.TransactWriteItem{ConditionCheck: &types.ConditionCheck{
		TableName:                 aws.String(t.Name),
		Key:                       key,
		ConditionExpression:       aws.String(condition),
		ExpressionAttributeValues: optionalValues(values),
	}}
}
