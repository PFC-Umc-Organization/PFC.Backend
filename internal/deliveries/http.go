package deliveries

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

type handlers struct{ svc *Service }

func Routes(r *httpapi.Router, svc *Service) {
	h := handlers{svc}

	r.Handle("GET", "/atividades", h.listActivities)
	r.Handle("POST", "/atividades", staff("atividade.criada", "", h.createActivity))
	r.Handle("PUT", "/atividades/:atividadeId", staff("atividade.atualizada", "atividadeId", h.updateActivity))
	r.Handle("DELETE", "/atividades/:atividadeId", staff("atividade.removida", "atividadeId", h.deleteActivity))
	r.Handle("POST", "/atividades/:atividadeId/campos", staff("atividade.campo_adicionado", "atividadeId", h.addField))
	r.Handle("DELETE", "/atividades/:atividadeId/campos/:campoId", staff("atividade.campo_removido", "atividadeId", h.removeField))
	r.Handle("GET", "/atividades/:atividadeId/entregas", staff("", "", h.submissionsOfActivity))
	r.Handle("GET", "/projetos/:projetoId/entregas", h.submissionsOfProject)
	r.Handle("PUT", "/projetos/:projetoId/entregas/:atividadeId", h.submit)
	r.Handle("DELETE", "/projetos/:projetoId/entregas/:atividadeId", staff("entrega.removida", "atividadeId", h.removeSubmission))
}

// staff admits admins and advisors. A denial is audited under auditAction,
// about the activity in the activityParam path parameter when given.
func staff(auditAction, activityParam string, h httpapi.Handler) httpapi.Handler {
	p := httpapi.Policy{
		Roles:       []actor.Role{actor.RoleAdmin, actor.RoleAdvisor},
		Message:     msgStaffOnly,
		AuditAction: auditAction,
	}
	if activityParam != "" {
		p.AuditResource = func(r *httpapi.Request) *audit.Resource { return activityResource(r.Param(activityParam)) }
	}
	return httpapi.Restrict(p, h)
}

func (h handlers) listActivities(ctx context.Context, _ *httpapi.Request) (httpapi.Response, error) {
	activities, err := h.svc.ListActivities(ctx)
	return httpapi.Reply(200, activities, err)
}

func (h handlers) createActivity(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ActivityInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	a, err := h.svc.CreateActivity(ctx, in)
	return httpapi.Reply(201, a, err)
}

func (h handlers) updateActivity(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ActivityInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	a, err := h.svc.UpdateActivity(ctx, r.Param("atividadeId"), in)
	return httpapi.Reply(200, a, err)
}

func (h handlers) deleteActivity(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("atividade removida", h.svc.DeleteActivity(ctx, r.Param("atividadeId")))
}

func (h handlers) addField(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in FieldInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	f, err := h.svc.AddField(ctx, r.Param("atividadeId"), in)
	return httpapi.Reply(201, f, err)
}

func (h handlers) removeField(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("campo removido", h.svc.RemoveField(ctx, r.Param("atividadeId"), r.Param("campoId")))
}

func (h handlers) submissionsOfActivity(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	submissions, err := h.svc.SubmissionsOfActivity(ctx, r.Param("atividadeId"))
	return httpapi.Reply(200, submissions, err)
}

func (h handlers) submissionsOfProject(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	submissions, err := h.svc.SubmissionsOfProject(ctx, r.Actor, r.Param("projetoId"))
	return httpapi.Reply(200, submissions, err)
}

func (h handlers) submit(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in SubmissionInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	sub, err := h.svc.Submit(ctx, r.Actor, r.Param("projetoId"), r.Param("atividadeId"), in)
	return httpapi.Reply(200, sub, err)
}

func (h handlers) removeSubmission(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("entrega removida", h.svc.RemoveSubmission(ctx, r.Param("projetoId"), r.Param("atividadeId")))
}
