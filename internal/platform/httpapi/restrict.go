package httpapi

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

// Policy is a route's role requirement. AuditAction, when set, records
// denials under that action.
type Policy struct {
	Roles       []actor.Role
	Message     string
	AuditAction string
}

func Restrict(p Policy, h Handler) Handler {
	return func(ctx context.Context, r *Request) (Response, error) {
		if !r.Actor.HasRole(p.Roles...) {
			if p.AuditAction != "" {
				audit.Record(ctx, audit.Event{Action: p.AuditAction, Result: audit.Failure, Reason: p.Message})
			}
			return Response{}, apperr.Forbidden(p.Message)
		}
		return h(ctx, r)
	}
}
