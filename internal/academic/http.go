package academic

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

type handlers struct{ svc *Service }

func Routes(r *httpapi.Router, svc *Service) {
	h := handlers{svc}

	r.Handle("POST", "/turmas", admin("turma.criada", h.createCohort))
	r.Handle("GET", "/turmas", h.listCohorts)
	r.Handle("PUT", "/turmas/:turmaId", admin("turma.atualizada", h.updateCohort))
	r.Handle("DELETE", "/turmas/:turmaId", admin("turma.removida", h.deleteCohort))

	r.Handle("GET", "/admin/students", admin("", h.listEnrollments))
	r.Handle("POST", "/admin/students", admin("matricula.provisionado", h.enroll))
	r.Handle("DELETE", "/admin/students", admin("matricula.removido", h.unenroll))

	r.Handle("POST", "/programas", admin("programa.criado", h.createProgram))
	r.Handle("GET", "/programas", h.listPrograms)
	r.Handle("PUT", "/programas/:programaId", admin("programa.atualizado", h.updateProgram))
	r.Handle("DELETE", "/programas/:programaId", admin("programa.removido", h.deleteProgram))
}

func admin(auditAction string, h httpapi.Handler) httpapi.Handler {
	return httpapi.Restrict(httpapi.Policy{
		Roles:       []actor.Role{actor.RoleAdmin},
		Message:     msgAdminOnly,
		AuditAction: auditAction,
	}, h)
}

func message(msg string) httpapi.Response {
	return httpapi.JSON(200, map[string]string{"mensagem": msg})
}

func (h handlers) createCohort(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in CohortInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	c, err := h.svc.CreateCohort(ctx, in)
	if err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.JSON(201, c), nil
}

func (h handlers) listCohorts(ctx context.Context, _ *httpapi.Request) (httpapi.Response, error) {
	cohorts, err := h.svc.ListCohorts(ctx)
	if err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.JSON(200, cohorts), nil
}

func (h handlers) updateCohort(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in CohortInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	if err := h.svc.UpdateCohort(ctx, r.Param("turmaId"), in); err != nil {
		return httpapi.Response{}, err
	}
	return message("turma atualizada"), nil
}

func (h handlers) deleteCohort(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	if err := h.svc.DeleteCohort(ctx, r.Param("turmaId")); err != nil {
		return httpapi.Response{}, err
	}
	return message("turma removida"), nil
}

func (h handlers) listEnrollments(ctx context.Context, _ *httpapi.Request) (httpapi.Response, error) {
	enrollments, err := h.svc.ListEnrollments(ctx)
	if err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.JSON(200, enrollments), nil
}

func (h handlers) enroll(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in EnrollmentRequest
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	result, err := h.svc.Enroll(ctx, in)
	if err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.JSON(200, result), nil
}

func (h handlers) unenroll(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in EnrollmentRequest
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	result, err := h.svc.Unenroll(ctx, in)
	if err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.JSON(200, result), nil
}

func (h handlers) createProgram(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ProgramInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	p, err := h.svc.CreateProgram(ctx, in)
	if err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.JSON(201, p), nil
}

func (h handlers) listPrograms(ctx context.Context, _ *httpapi.Request) (httpapi.Response, error) {
	programs, err := h.svc.ListPrograms(ctx)
	if err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.JSON(200, programs), nil
}

func (h handlers) updateProgram(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ProgramInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	if err := h.svc.UpdateProgram(ctx, r.Param("programaId"), in); err != nil {
		return httpapi.Response{}, err
	}
	return message("programa atualizado"), nil
}

func (h handlers) deleteProgram(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	if err := h.svc.DeleteProgram(ctx, r.Param("programaId")); err != nil {
		return httpapi.Response{}, err
	}
	return message("programa removido"), nil
}
