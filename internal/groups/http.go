package groups

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

type handlers struct{ svc *Service }

func Routes(r *httpapi.Router, svc *Service) {
	h := handlers{svc}

	r.Handle("GET", "/meu-pfc", httpapi.Student(msgStudentsOnly, h.myPFC, httpapi.NeedRGM(msgNoRGM)))
	r.Handle("POST", "/meu-pfc", httpapi.Student(msgStudentsOnlyCreate, h.createOwnProject, httpapi.Audit("projeto.criado_pelo_aluno"), httpapi.NeedRGM(msgNoRGM)))
	r.Handle("POST", "/programas/:programaId/projetos", httpapi.Admin(msgAdminOnly, h.createProject, httpapi.Audit("projeto.criado")))
	r.Handle("GET", "/programas/:programaId/projetos", httpapi.Staff(msgStaffOnly, h.projectsOfProgram))
	r.Handle("GET", "/projetos/:projetoId", h.projectDetail)
	r.Handle("PUT", "/projetos/:projetoId", httpapi.Admin(msgAdminOnly, h.updateProject, httpapi.Audit("projeto.atualizado")))
	r.Handle("DELETE", "/projetos/:projetoId", httpapi.Admin(msgAdminOnly, h.deleteProject, httpapi.Audit("projeto.removido")))
	r.Handle("PUT", "/projetos/:projetoId/orientador", httpapi.Admin(msgAdminOnly, h.assignAdvisor, httpapi.Audit("projeto.orientador_associado")))
	r.Handle("DELETE", "/projetos/:projetoId/orientador", httpapi.Admin(msgAdminOnly, h.removeAdvisor, httpapi.Audit("projeto.orientador_removido")))
	r.Handle("PUT", "/projetos/:projetoId/integrantes", httpapi.Admin(msgAdminOnly, h.addMember, httpapi.Audit("projeto.integrante_adicionado")))
	r.Handle("DELETE", "/projetos/:projetoId/integrantes", httpapi.Admin(msgAdminOnly, h.removeMember, httpapi.Audit("projeto.integrante_removido")))

	r.Handle("GET", "/meus-convites", httpapi.Student(msgStudentsOnly, h.myInvitations, httpapi.NeedRGM(msgNoRGM)))
	r.Handle("POST", "/meus-convites/:projetoId/aceitar", httpapi.Student(msgStudentsOnly, h.acceptInvitation, httpapi.NeedRGM(msgNoRGM)))
	r.Handle("POST", "/meus-convites/:projetoId/recusar", httpapi.Student(msgStudentsOnly, h.declineInvitation, httpapi.NeedRGM(msgNoRGM)))
	r.Handle("GET", "/projetos/:projetoId/convites", h.projectInvitations)
	r.Handle("POST", "/projetos/:projetoId/convites", h.invite)
	r.Handle("DELETE", "/projetos/:projetoId/convites/:rgm", h.cancelInvitation)
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

func (h handlers) myPFC(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	mine, err := h.svc.MyPFC(ctx, r.Actor)
	return httpapi.Reply(200, mine, err)
}

func (h handlers) projectsOfProgram(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	projects, err := h.svc.ProjectsOfProgram(ctx, r.Actor, r.Param("programaId"))
	return httpapi.Reply(200, projects, err)
}

func (h handlers) projectDetail(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	d, err := h.svc.Detail(ctx, r.Actor, r.Param("projetoId"))
	return httpapi.Reply(200, d, err)
}

func (h handlers) updateProject(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ProjectInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Projeto atualizado.", h.svc.UpdateProject(ctx, r.Param("projetoId"), in))
}

func (h handlers) deleteProject(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("Projeto excluído.", h.svc.DeleteProject(ctx, r.Param("projetoId")))
}

func (h handlers) assignAdvisor(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in AdvisorInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Orientador definido.", h.svc.AssignAdvisor(ctx, r.Param("projetoId"), in))
}

func (h handlers) removeAdvisor(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("Orientador removido.", h.svc.RemoveAdvisor(ctx, r.Param("projetoId")))
}

func (h handlers) addMember(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in MemberInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Aluno incluído no projeto.", h.svc.AddMember(ctx, r.Param("projetoId"), in))
}

func (h handlers) removeMember(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in MemberInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Aluno removido do projeto.", h.svc.RemoveMember(ctx, r.Param("projetoId"), in))
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
	return httpapi.Message("Convite recusado.", h.svc.DeclineInvitation(ctx, r.Actor.RGM(), r.Param("projetoId")))
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
	return httpapi.Message("Convite cancelado.", h.svc.CancelInvitation(ctx, r.Actor, r.Param("projetoId"), r.Param("rgm")))
}
