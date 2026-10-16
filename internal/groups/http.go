package groups

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

type handlers struct{ svc *Service }

func Routes(r *httpapi.Router, svc *Service) {
	h := handlers{svc}

	r.Handle("POST", "/meu-pfc", student(msgStudentsOnlyCreate, "projeto.criado_pelo_aluno", h.createOwnProject))
	r.Handle("POST", "/programas/:programaId/projetos", admin("projeto.criado", h.createProject))
	r.Handle("GET", "/programas/:programaId/projetos", h.projectsOfProgram)
	r.Handle("PUT", "/projetos/:projetoId", admin("projeto.atualizado", h.updateProject))
	r.Handle("DELETE", "/projetos/:projetoId", admin("projeto.removido", h.deleteProject))
	r.Handle("PUT", "/projetos/:projetoId/orientador", admin("projeto.orientador_associado", h.assignAdvisor))
	r.Handle("DELETE", "/projetos/:projetoId/orientador", admin("projeto.orientador_removido", h.removeAdvisor))
	r.Handle("PUT", "/projetos/:projetoId/integrantes", admin("projeto.integrante_adicionado", h.addMember))
	r.Handle("DELETE", "/projetos/:projetoId/integrantes", admin("projeto.integrante_removido", h.removeMember))

	r.Handle("GET", "/meus-convites", student(msgStudentsOnly, "", h.myInvitations))
	r.Handle("POST", "/meus-convites/:projetoId/aceitar", student(msgStudentsOnly, "", h.acceptInvitation))
	r.Handle("POST", "/meus-convites/:projetoId/recusar", student(msgStudentsOnly, "", h.declineInvitation))
	r.Handle("GET", "/projetos/:projetoId/convites", h.projectInvitations)
	r.Handle("POST", "/projetos/:projetoId/convites", h.invite)
	r.Handle("DELETE", "/projetos/:projetoId/convites/:rgm", h.cancelInvitation)
}

func admin(auditAction string, h httpapi.Handler) httpapi.Handler {
	return httpapi.Restrict(httpapi.Policy{
		Roles:       []actor.Role{actor.RoleAdmin},
		Message:     msgAdminOnly,
		AuditAction: auditAction,
	}, h)
}

// student admits anyone who is not academic staff and has an RGM; students
// who signed themselves up may carry no role at all.
func student(staffMessage, auditAction string, h httpapi.Handler) httpapi.Handler {
	deny := func(ctx context.Context, reason, msg string) (httpapi.Response, error) {
		if auditAction != "" {
			audit.Record(ctx, audit.Event{Action: auditAction, Result: audit.Failure, Reason: reason})
		}
		return httpapi.Response{}, apperr.Forbidden(msg)
	}
	return func(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
		if isStaff(r.Actor) {
			return deny(ctx, "rota exclusiva de aluno", staffMessage)
		}
		if r.Actor.RGM() == "" {
			return deny(ctx, "conta sem RGM", msgNoRGM)
		}
		return h(ctx, r)
	}
}

func (h handlers) createOwnProject(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ProjectInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	p, err := h.svc.CreateOwnProject(ctx, r.Actor, in)
	return httpapi.Reply(201, p, err)
}

func (h handlers) createProject(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ProjectInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	p, err := h.svc.CreateProject(ctx, r.Param("programaId"), in)
	return httpapi.Reply(201, p, err)
}

func (h handlers) projectsOfProgram(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	projects, err := h.svc.ProjectsOfProgram(ctx, r.Param("programaId"))
	return httpapi.Reply(200, projects, err)
}

func (h handlers) updateProject(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ProjectInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("projeto atualizado", h.svc.UpdateProject(ctx, r.Param("projetoId"), in))
}

func (h handlers) deleteProject(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("projeto removido", h.svc.DeleteProject(ctx, r.Param("projetoId")))
}

func (h handlers) assignAdvisor(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in AdvisorInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("orientador associado", h.svc.AssignAdvisor(ctx, r.Param("projetoId"), in))
}

func (h handlers) removeAdvisor(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("orientador removido", h.svc.RemoveAdvisor(ctx, r.Param("projetoId")))
}

func (h handlers) addMember(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in MemberInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("aluno associado ao projeto", h.svc.AddMember(ctx, r.Param("projetoId"), in))
}

func (h handlers) removeMember(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in MemberInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("aluno removido do projeto", h.svc.RemoveMember(ctx, r.Param("projetoId"), in))
}

func (h handlers) myInvitations(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	invitations, err := h.svc.InvitationsOf(ctx, r.Actor.RGM())
	return httpapi.Reply(200, invitations, err)
}

func (h handlers) acceptInvitation(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	p, err := h.svc.AcceptInvitation(ctx, r.Actor.RGM(), r.Param("projetoId"))
	return httpapi.Reply(200, p, err)
}

func (h handlers) declineInvitation(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("convite recusado", h.svc.DeclineInvitation(ctx, r.Actor.RGM(), r.Param("projetoId")))
}

func (h handlers) projectInvitations(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	invitations, err := h.svc.InvitationsOfProject(ctx, r.Actor, r.Param("projetoId"))
	return httpapi.Reply(200, invitations, err)
}

func (h handlers) invite(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in MemberInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	inv, err := h.svc.Invite(ctx, r.Actor, r.Param("projetoId"), in)
	return httpapi.Reply(201, inv, err)
}

func (h handlers) cancelInvitation(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("convite cancelado", h.svc.CancelInvitation(ctx, r.Actor, r.Param("projetoId"), r.Param("rgm")))
}
