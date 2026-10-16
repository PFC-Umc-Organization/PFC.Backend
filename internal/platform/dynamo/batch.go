package dynamo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	BatchGetSize   = 100
	BatchWriteSize = 25

	batchAttempts = 5
	firstBackoff  = 50 * time.Millisecond
)

// retryUnprocessed calls send until it reports nothing pending, with
// exponential backoff: under throttling DynamoDB returns part of a batch as
// unprocessed instead of failing.
func retryUnprocessed(ctx context.Context, send func() (pending int, err error)) error {
	backoff := firstBackoff
	for attempt := 0; ; attempt++ {
		pending, err := send()
		if err != nil || pending == 0 {
			return err
		}
		if attempt == batchAttempts-1 {
			return fmt.Errorf("%d requests still unprocessed after %d attempts", pending, batchAttempts)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
}

// BatchGet returns the items that exist, in no particular order.
func (t *Table) BatchGet(ctx context.Context, keys []Item, projection ...string) ([]Item, error) {
	var items []Item
	for start := 0; start < len(keys); start += BatchGetSize {
		req := types.KeysAndAttributes{Keys: keys[start:min(start+BatchGetSize, len(keys))]}
		if len(projection) > 0 {
			req.ProjectionExpression = aws.String(strings.Join(projection, ", "))
		}
		pending := map[string]types.KeysAndAttributes{t.Name: req}
		err := retryUnprocessed(ctx, func() (int, error) {
			out, err := t.Client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{RequestItems: pending})
			if err != nil {
				return 0, err
			}
			items = append(items, out.Responses[t.Name]...)
			pending = out.UnprocessedKeys
			return len(pending[t.Name].Keys), nil
		})
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (t *Table) BatchWrite(ctx context.Context, writes []types.WriteRequest) error {
	for start := 0; start < len(writes); start += BatchWriteSize {
		pending := writes[start:min(start+BatchWriteSize, len(writes))]
		err := retryUnprocessed(ctx, func() (int, error) {
			out, err := t.Client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
				RequestItems: map[string][]types.WriteRequest{t.Name: pending},
			})
			if err != nil {
				return 0, err
			}
			pending = out.UnprocessedItems[t.Name]
			return len(pending), nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *Table) DeleteKeys(ctx context.Context, keys []Item) error {
	writes := make([]types.WriteRequest, 0, len(keys))
	for _, k := range keys {
		writes = append(writes, types.WriteRequest{DeleteRequest: &types.DeleteRequest{Key: k}})
	}
	return t.BatchWrite(ctx, writes)
}
