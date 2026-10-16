package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

type Handler func(ctx context.Context, r *Request) (Response, error)

type route struct {
	segments []string
	handler  Handler
}

type Router struct {
	routes map[string][]route
}

func NewRouter() *Router {
	return &Router{routes: map[string][]route{}}
}

// Handle registers a route; segments starting with ":" are parameters.
func (rt *Router) Handle(method, pattern string, h Handler) {
	rt.routes[method] = append(rt.routes[method], route{segments: split(pattern), handler: h})
}

func (rt *Router) Lambda(ctx context.Context, ev events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if ev.HTTPMethod == "OPTIONS" {
		return events.APIGatewayProxyResponse{StatusCode: 204, Headers: corsHeaders}, nil
	}
	return ToLambda(rt.serve(ctx, ev)), nil
}

func (rt *Router) serve(ctx context.Context, ev events.APIGatewayProxyRequest) Response {
	path := split(ev.Path)
	for _, rte := range rt.routes[ev.HTTPMethod] {
		params, ok := match(rte.segments, path)
		if !ok {
			continue
		}
		if ev.PathParameters == nil {
			ev.PathParameters = map[string]string{}
		}
		for k, v := range params {
			ev.PathParameters[k] = v
		}
		req := newRequest(ev)
		ctx = audit.WithOrigin(ctx, audit.Origin{
			Actor:     audit.Actor{Sub: req.Actor.Sub, Role: string(req.Actor.Role), Email: req.Actor.Email, IP: req.SourceIP},
			RequestID: req.RequestID,
		})
		return run(ctx, rte.handler, req)
	}
	return Error(404, "rota não encontrada")
}

func run(ctx context.Context, h Handler, req *Request) (resp Response) {
	defer func() {
		if p := recover(); p != nil {
			slog.ErrorContext(ctx, "handler panicked", "method", req.Method, "path", req.Path,
				"panic", fmt.Sprint(p), "stack", string(debug.Stack()))
			resp = Error(500, msgInternal)
		}
	}()
	resp, err := h(ctx, req)
	if err != nil {
		return errorResponse(ctx, req, err)
	}
	return resp
}

const msgInternal = "erro interno — tente novamente em instantes"

var statusOf = map[apperr.Kind]int{
	apperr.KindInvalid:      400,
	apperr.KindUnauthorized: 401,
	apperr.KindForbidden:    403,
	apperr.KindNotFound:     404,
	apperr.KindConflict:     409,
	apperr.KindUnavailable:  502,
	apperr.KindInternal:     500,
}

func errorResponse(ctx context.Context, req *Request, err error) Response {
	e, ok := apperr.As(err)
	if !ok {
		slog.ErrorContext(ctx, "request failed", "method", req.Method, "path", req.Path, "error", err)
		return Error(500, msgInternal)
	}
	status := statusOf[e.Kind]
	if status >= 500 {
		slog.ErrorContext(ctx, "request failed", "method", req.Method, "path", req.Path, "error", err)
	}
	return Error(status, e.Message)
}

func split(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func match(pattern, actual []string) (map[string]string, bool) {
	if len(pattern) != len(actual) {
		return nil, false
	}
	params := map[string]string{}
	for i, seg := range pattern {
		if name, ok := strings.CutPrefix(seg, ":"); ok {
			params[name] = actual[i]
		} else if seg != actual[i] {
			return nil, false
		}
	}
	return params, true
}
