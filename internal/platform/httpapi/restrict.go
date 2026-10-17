package httpapi

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

// Policy is a route's role requirement. AuditAction, when set, records
// denials under that action, about AuditResource when that is set too.
type Policy struct {
	Roles         []actor.Role
	Message       string
	AuditAction   string
	AuditResource func(*Request) *audit.Resource
}

func Restrict(p Policy, h Handler) Handler {
	return func(ctx context.Context, r *Request) (Response, error) {
		if !r.Actor.HasRole(p.Roles...) {
			if p.AuditAction != "" {
				ev := audit.Event{Action: p.AuditAction, Result: audit.Failure, Reason: p.Message}
				if p.AuditResource != nil {
					ev.Resource = p.AuditResource(r)
				}
				audit.Record(ctx, ev)
			}
			return Response{}, apperr.Forbidden(p.Message)
		}
		return h(ctx, r)
	}
}
