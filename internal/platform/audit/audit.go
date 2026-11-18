package audit

import (
	"context"
	"io"
	"log/slog"
	"os"
)

// ---------------------------------------------------------------------------
// Modelo
// ---------------------------------------------------------------------------

const (
	Success = "sucesso"
	Failure = "falha"
)

// Ator
type Actor struct {
	Sub   string `json:"sub,omitempty"`
	Role  string `json:"perfil,omitempty"`
	Email string `json:"email,omitempty"`
	IP    string `json:"ip,omitempty"`
}

// Recurso
type Resource struct {
	Type string `json:"tipo"`
	ID   string `json:"id,omitempty"`
}

// Evento
type Event struct {
	Action   string
	Result   string
	Reason   string
	Actor    *Actor
	Resource *Resource
	Details  map[string]any
}

// Origem
type Origin struct {
	Actor     Actor
	RequestID string
}

// ---------------------------------------------------------------------------
// Origem
// ---------------------------------------------------------------------------

// Chave
type originKey struct{}

// Contexto
func WithOrigin(ctx context.Context, o Origin) context.Context {
	return context.WithValue(ctx, originKey{}, o)
}

// ---------------------------------------------------------------------------
// Registro
// ---------------------------------------------------------------------------

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

// Configuração
func SetOutput(w io.Writer) (restore func()) {
	previous := logger
	logger = slog.New(slog.NewJSONHandler(w, nil))
	return func() { logger = previous }
}

// Registro
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
