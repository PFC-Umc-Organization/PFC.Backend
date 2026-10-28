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
	r.Handle("POST", "/atividades", coordination("atividade.criada", "", h.createActivity))
	r.Handle("PUT", "/atividades/:atividadeId", coordination("atividade.atualizada", "atividadeId", h.updateActivity))
	r.Handle("DELETE", "/atividades/:atividadeId", coordination("atividade.removida", "atividadeId", h.deleteActivity))
	r.Handle("POST", "/atividades/:atividadeId/campos", coordination("atividade.campo_adicionado", "atividadeId", h.addField))
	r.Handle("DELETE", "/atividades/:atividadeId/campos/:campoId", coordination("atividade.campo_removido", "atividadeId", h.removeField))
	r.Handle("GET", "/atividades/:atividadeId/entregas", httpapi.Restrict(httpapi.Policy{
		Roles:   []actor.Role{actor.RoleAdmin},
		Message: msgAdminOnly,
	}, h.submissionsOfActivity))
	r.Handle("GET", "/programas/:programaId/entregas", staff("", "", h.submissionsOfProgram))
	r.Handle("GET", "/projetos/:projetoId/entregas", h.submissionsOfProject)
	r.Handle("PUT", "/projetos/:projetoId/entregas/:atividadeId", h.submit)
	r.Handle("DELETE", "/projetos/:projetoId/entregas/:atividadeId", staff("entrega.removida", "atividadeId", h.removeSubmission))
	r.Handle("PUT", "/projetos/:projetoId/entregas/:atividadeId/validacao", h.validate)
	r.Handle("POST", "/projetos/:projetoId/entregas/:atividadeId/arquivos", h.requestUpload)
	r.Handle("GET", "/projetos/:projetoId/entregas/:atividadeId/arquivos/:campoId", h.downloadFile)

	r.Handle("GET", "/programas/:programaId/penalidades", staff("", "", h.penaltiesOfProgram))
	r.Handle("GET", "/projetos/:projetoId/penalidades", h.penaltiesOfProject)
	r.Handle("POST", "/projetos/:projetoId/penalidades", staff("penalidade.aplicada", "", h.applyPenalty))
	r.Handle("POST", "/projetos/:projetoId/penalidades/:penalidadeId/perdao", staff("penalidade.perdoada", "", h.waivePenalty))
}

// staff admits admins and advisors. A denial is audited under auditAction,
// about the activity in the activityParam path parameter when given.
func staff(auditAction, activityParam string, h httpapi.Handler) httpapi.Handler {
	return restrict([]actor.Role{actor.RoleAdmin, actor.RoleAdvisor}, msgStaffOnly, auditAction, activityParam, h)
}

// coordination guards the activities: they are global, so changing one
// changes every group's deliverables and deadline penalties.
func coordination(auditAction, activityParam string, h httpapi.Handler) httpapi.Handler {
	return restrict([]actor.Role{actor.RoleAdmin}, msgActivitiesAdminOnly, auditAction, activityParam, h)
}

func restrict(roles []actor.Role, message, auditAction, activityParam string, h httpapi.Handler) httpapi.Handler {
	p := httpapi.Policy{Roles: roles, Message: message, AuditAction: auditAction}
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
	return httpapi.Message("Atividade excluída.", h.svc.DeleteActivity(ctx, r.Param("atividadeId")))
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
	return httpapi.Message("Campo removido.", h.svc.RemoveField(ctx, r.Param("atividadeId"), r.Param("campoId")))
}

func (h handlers) submissionsOfActivity(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	submissions, err := h.svc.SubmissionsOfActivity(ctx, r.Param("atividadeId"))
	return httpapi.Reply(200, submissions, err)
}

func (h handlers) submissionsOfProgram(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	submissions, err := h.svc.SubmissionsOfProgram(ctx, r.Actor, r.Param("programaId"))
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
	return httpapi.Message("Entrega devolvida ao grupo.", h.svc.RemoveSubmission(ctx, r.Actor, r.Param("projetoId"), r.Param("atividadeId")))
}

func (h handlers) validate(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ValidationInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	sub, err := h.svc.Validate(ctx, r.Actor, r.Param("projetoId"), r.Param("atividadeId"), in)
	return httpapi.Reply(200, sub, err)
}

func (h handlers) requestUpload(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in UploadRequest
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	ticket, err := h.svc.RequestUpload(ctx, r.Actor, r.Param("projetoId"), r.Param("atividadeId"), in)
	return httpapi.Reply(201, ticket, err)
}

func (h handlers) downloadFile(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	d, err := h.svc.DownloadFile(ctx, r.Actor, r.Param("projetoId"), r.Param("atividadeId"), r.Param("campoId"))
	return httpapi.Reply(200, d, err)
}

func (h handlers) penaltiesOfProgram(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	penalties, err := h.svc.PenaltiesOfProgram(ctx, r.Actor, r.Param("programaId"))
	return httpapi.Reply(200, penalties, err)
}

func (h handlers) penaltiesOfProject(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	penalties, err := h.svc.PenaltiesOfProject(ctx, r.Actor, r.Param("projetoId"))
	return httpapi.Reply(200, penalties, err)
}

func (h handlers) applyPenalty(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in PenaltyInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	penalties, err := h.svc.ApplyPenalty(ctx, r.Actor, r.Param("projetoId"), in)
	return httpapi.Reply(201, penalties, err)
}

func (h handlers) waivePenalty(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in WaiverInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	penalties, err := h.svc.WaivePenalty(ctx, r.Actor, r.Param("projetoId"), r.Param("penalidadeId"), in)
	return httpapi.Reply(200, penalties, err)
}
