package httpapi

import (
	"context"

	"github.com/aws/aws-lambda-go/events"
)

// LegacyHandler is the handler shape of the packages not yet migrated to
// the modules; Legacy goes away with them.
type LegacyHandler func(ctx context.Context, ev events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error)

func Legacy(h LegacyHandler) Handler {
	return func(ctx context.Context, r *Request) (Response, error) {
		resp, err := h(ctx, r.event)
		return Response{Status: resp.StatusCode, Body: resp.Body}, err
	}
}
