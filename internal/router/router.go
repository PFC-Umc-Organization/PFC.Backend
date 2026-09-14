package router

import (
	"context"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

type Handler func(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error)

type registeredRoute struct {
	method   string
	segments []string
	handler  Handler
}

type Router struct {
	routes []registeredRoute
}

func New() *Router {
	return &Router{}
}


func (r *Router) Handle(method, path string, h Handler) {
	r.routes = append(r.routes, registeredRoute{
		method:   method,
		segments: splitPath(path),
		handler:  h,
	})
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return []string{}
	}
	return strings.Split(trimmed, "/")
}

func (r *Router) Dispatch(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if req.HTTPMethod == "OPTIONS" {
		return events.APIGatewayProxyResponse{
			StatusCode: 204,
			Headers: map[string]string{
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Headers": "Content-Type,Authorization",
				"Access-Control-Allow-Methods": "GET,POST,PUT,DELETE,OPTIONS",
			},
		}, nil
	}

	requestSegments := splitPath(req.Path)

	for _, route := range r.routes {
		if route.method != req.HTTPMethod {
			continue
		}
		params, ok := match(route.segments, requestSegments)
		if !ok {
			continue
		}
		if req.PathParameters == nil {
			req.PathParameters = map[string]string{}
		}
		for k, v := range params {
			req.PathParameters[k] = v
		}
		return route.handler(ctx, req)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: 404,
		Body:       `{"erro":"rota não encontrada"}`,
	}, nil
}


func match(pattern, actual []string) (map[string]string, bool) {
	if len(pattern) != len(actual) {
		return nil, false
	}
	params := map[string]string{}
	for i, seg := range pattern {
		if strings.HasPrefix(seg, ":") {
			params[seg[1:]] = actual[i]
			continue
		}
		if seg != actual[i] {
			return nil, false
		}
	}
	return params, true
}
