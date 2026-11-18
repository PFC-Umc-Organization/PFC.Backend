package httpapi

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

// ---------------------------------------------------------------------------
// Política
// ---------------------------------------------------------------------------

// Política
type Policy struct {
	Roles         []actor.Role
	Message       string
	AuditAction   string
	AuditResource func(*Request) *audit.Resource

	rgmMessage string
}

// Opção
type Option func(*Policy)

// Auditoria
func Audit(action string) Option {
	return func(p *Policy) { p.AuditAction = action }
}

// Auditoria
func AuditParam(resourceType, param string) Option {
	return func(p *Policy) {
		p.AuditResource = func(r *Request) *audit.Resource {
			return &audit.Resource{Type: resourceType, ID: r.Param(param)}
		}
	}
}

// Exigência
func NeedRGM(message string) Option {
	return func(p *Policy) { p.rgmMessage = message }
}

// Montagem
func build(roles []actor.Role, msg string, opts []Option) Policy {
	p := Policy{Roles: roles, Message: msg}
	for _, o := range opts {
		o(&p)
	}
	return p
}

// ---------------------------------------------------------------------------
// Perfis
// ---------------------------------------------------------------------------

// Restrição
func Admin(msg string, h Handler, opts ...Option) Handler {
	return Restrict(build([]actor.Role{actor.RoleAdmin}, msg, opts), h)
}

// Restrição
func Staff(msg string, h Handler, opts ...Option) Handler {
	return Restrict(build([]actor.Role{actor.RoleAdmin, actor.RoleAdvisor}, msg, opts), h)
}

// Restrição
func Student(msg string, h Handler, opts ...Option) Handler {
	p := build(nil, msg, opts)
	deny := func(ctx context.Context, reason, message string) (Response, error) {
		if p.AuditAction != "" {
			audit.Record(ctx, audit.Event{Action: p.AuditAction, Result: audit.Failure, Reason: reason})
		}
		return Response{}, apperr.Forbidden(message)
	}
	return func(ctx context.Context, r *Request) (Response, error) {
		if r.Actor.HasRole(actor.RoleAdmin, actor.RoleAdvisor) {
			return deny(ctx, "rota exclusiva de aluno", p.Message)
		}
		if p.rgmMessage != "" && r.Actor.RGM() == "" {
			return deny(ctx, "conta sem RGM", p.rgmMessage)
		}
		return h(ctx, r)
	}
}

// Restrição
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
