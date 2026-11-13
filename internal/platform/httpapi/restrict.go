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

	rgmMessage string
}

// Option adjusts the Policy built by Admin, Staff and Student.
type Option func(*Policy)

// Audit records denials under action.
func Audit(action string) Option {
	return func(p *Policy) { p.AuditAction = action }
}

// AuditParam makes the audited denial about the resource of the given type
// whose id is the path parameter param.
func AuditParam(resourceType, param string) Option {
	return func(p *Policy) {
		p.AuditResource = func(r *Request) *audit.Resource {
			return &audit.Resource{Type: resourceType, ID: r.Param(param)}
		}
	}
}

// NeedRGM, for Student routes, denies accounts whose e-mail yields no RGM.
func NeedRGM(message string) Option {
	return func(p *Policy) { p.rgmMessage = message }
}

func build(roles []actor.Role, msg string, opts []Option) Policy {
	p := Policy{Roles: roles, Message: msg}
	for _, o := range opts {
		o(&p)
	}
	return p
}

// Admin admits the coordination only.
func Admin(msg string, h Handler, opts ...Option) Handler {
	return Restrict(build([]actor.Role{actor.RoleAdmin}, msg, opts), h)
}

// Staff admits the coordination and advisors.
func Staff(msg string, h Handler, opts ...Option) Handler {
	return Restrict(build([]actor.Role{actor.RoleAdmin, actor.RoleAdvisor}, msg, opts), h)
}

// Student refuses the staff with msg. An account without role counts as a
// student (self-registered accounts carry none).
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
