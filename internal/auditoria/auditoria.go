// Package auditoria is the transitional bridge to platform/audit for the
// packages not yet moved into modules. Removed in phase 7 of the refactor.
package auditoria

import (
	"context"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

type Ator struct {
	Sub, Perfil, Email, IP string
}

type Recurso struct {
	Tipo, ID string
}

type Evento struct {
	Acao      string
	Resultado string
	Motivo    string
	Ator      *Ator
	Recurso   *Recurso
	Detalhes  map[string]any
}

func Registrar(ctx context.Context, ev Evento, req events.APIGatewayProxyRequest) {
	ctx = audit.WithOrigin(ctx, audit.Origin{Actor: actorOf(req), RequestID: req.RequestContext.RequestID})
	out := audit.Event{Action: ev.Acao, Result: ev.Resultado, Reason: ev.Motivo, Details: ev.Detalhes}
	if a := ev.Ator; a != nil {
		out.Actor = &audit.Actor{Sub: a.Sub, Role: a.Perfil, Email: a.Email, IP: a.IP}
	}
	if r := ev.Recurso; r != nil {
		out.Resource = &audit.Resource{Type: r.Tipo, ID: r.ID}
	}
	audit.Record(ctx, out)
}
