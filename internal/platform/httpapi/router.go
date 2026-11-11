package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

type Handler func(ctx context.Context, r *Request) (Response, error)

type route struct {
	segments []string
	nParams  int
	public   bool
	handler  Handler
}

type Router struct {
	routes map[string][]route
}

func NewRouter() *Router {
	return &Router{routes: map[string][]route{}}
}

// Handle registers a route that requires an authenticated caller; segments
// starting with ":" are parameters. A request that reaches it without
// authorizer claims is refused with 401 before the handler runs.
func (rt *Router) Handle(method, pattern string, h Handler) {
	rt.add(method, pattern, h, false)
}

// Public registers a route that is served without authentication.
func (rt *Router) Public(method, pattern string, h Handler) {
	rt.add(method, pattern, h, true)
}

func (rt *Router) add(method, pattern string, h Handler, public bool) {
	segments := split(pattern)
	n := 0
	for _, seg := range segments {
		if strings.HasPrefix(seg, ":") {
			n++
		}
	}
	rt.routes[method] = append(rt.routes[method], route{segments: segments, nParams: n, public: public, handler: h})
}

func (rt *Router) Lambda(ctx context.Context, ev events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if ev.HTTPMethod == "OPTIONS" {
		return events.APIGatewayProxyResponse{StatusCode: 204, Headers: corsHeaders}, nil
	}
	return ToLambda(rt.serve(ctx, ev)), nil
}

func (rt *Router) serve(ctx context.Context, ev events.APIGatewayProxyRequest) Response {
	path := split(ev.Path)
	if slices.Contains(path, "") {
		return Error(404, msgNotFound)
	}
	for _, rte := range rt.routes[ev.HTTPMethod] {
		if !match(rte.segments, path) {
			continue
		}
		req := newRequest(ev, params(rte, path, ev.PathParameters))
		if !rte.public && req.Actor.Sub == "" {
			slog.WarnContext(ctx, "unauthenticated request reached lambda", "method", req.Method, "path", req.Path, "requestId", req.RequestID)
			return Error(401, "Sessão expirada. Entre novamente.")
		}
		ctx = audit.WithOrigin(ctx, audit.Origin{
			Actor:     audit.Actor{Sub: req.Actor.Sub, Role: string(req.Actor.Role), Email: req.Actor.Email, IP: req.SourceIP},
			RequestID: req.RequestID,
		})
		return run(ctx, rte.handler, req)
	}
	return Error(404, msgNotFound)
}

const msgNotFound = "Endereço não encontrado."

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

const msgInternal = "Ocorreu um erro inesperado. Tente novamente em instantes."

var statusOf = map[apperr.Kind]int{
	apperr.KindInvalid:         400,
	apperr.KindUnauthorized:    401,
	apperr.KindForbidden:       403,
	apperr.KindNotFound:        404,
	apperr.KindConflict:        409,
	apperr.KindTooManyRequests: 429,
	apperr.KindUnavailable:     502,
	apperr.KindInternal:        500,
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

func match(pattern, actual []string) bool {
	if len(pattern) != len(actual) {
		return false
	}
	for i, seg := range pattern {
		if !strings.HasPrefix(seg, ":") && seg != actual[i] {
			return false
		}
	}
	return true
}

// params extracts the path parameters of the winning route. Parameters
// supplied by the API Gateway are kept; the route's own take precedence.
func params(rte route, actual []string, gateway map[string]string) map[string]string {
	if rte.nParams == 0 {
		return gateway
	}
	out := make(map[string]string, rte.nParams+len(gateway))
	for k, v := range gateway {
		out[k] = v
	}
	for i, seg := range rte.segments {
		if name, ok := strings.CutPrefix(seg, ":"); ok {
			out[name] = actual[i]
		}
	}
	return out
}
