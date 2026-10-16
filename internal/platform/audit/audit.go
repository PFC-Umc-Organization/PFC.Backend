// Package audit writes who-did-what events as JSON lines tagged
// tipo=auditoria in the Lambda log group. The field names and values are
// the log's data format and stay in Portuguese.
package audit

import (
	"context"
	"io"
	"log/slog"
	"os"
)

const (
	Success = "sucesso"
	Failure = "falha"
)

type Actor struct {
	Sub   string `json:"sub,omitempty"`
	Role  string `json:"perfil,omitempty"`
	Email string `json:"email,omitempty"`
	IP    string `json:"ip,omitempty"`
}

type Resource struct {
	Type string `json:"tipo"`
	ID   string `json:"id,omitempty"`
}

// Event.Actor is normally left nil and taken from the request origin.
// Public routes (login, sign-up) have no session and set it explicitly.
type Event struct {
	Action   string
	Result   string
	Reason   string
	Actor    *Actor
	Resource *Resource
	Details  map[string]any
}

type Origin struct {
	Actor     Actor
	RequestID string
}

type originKey struct{}

func WithOrigin(ctx context.Context, o Origin) context.Context {
	return context.WithValue(ctx, originKey{}, o)
}

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

// SetOutput redirects the events, for tests; restore puts stdout back.
func SetOutput(w io.Writer) (restore func()) {
	previous := logger
	logger = slog.New(slog.NewJSONHandler(w, nil))
	return func() { logger = previous }
}

func Record(ctx context.Context, ev Event) {
	origin, _ := ctx.Value(originKey{}).(Origin)
	actor := origin.Actor
	if ev.Actor != nil {
		actor = *ev.Actor
		if actor.IP == "" {
			actor.IP = origin.Actor.IP
		}
	}

	attrs := []any{
		slog.String("tipo", "auditoria"),
		slog.String("acao", ev.Action),
		slog.String("resultado", ev.Result),
		slog.Any("ator", actor),
	}
	if ev.Reason != "" {
		attrs = append(attrs, slog.String("motivo", ev.Reason))
	}
	if ev.Resource != nil {
		attrs = append(attrs, slog.Any("recurso", ev.Resource))
	}
	if len(ev.Details) > 0 {
		attrs = append(attrs, slog.Any("detalhes", ev.Details))
	}
	if origin.RequestID != "" {
		attrs = append(attrs, slog.String("requestId", origin.RequestID))
	}
	logger.InfoContext(ctx, "audit_event", attrs...)
}
