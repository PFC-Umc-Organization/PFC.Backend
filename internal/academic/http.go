package academic

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

type handlers struct{ svc *Service }

func Routes(r *httpapi.Router, svc *Service) {
	h := handlers{svc}

	r.Handle("POST", "/cursos", httpapi.Admin(msgAdminOnly, h.createCourse, httpapi.Audit("curso.criado")))
	r.Handle("GET", "/cursos", h.listCourses)
	r.Handle("PUT", "/cursos/:cursoId", httpapi.Admin(msgAdminOnly, h.updateCourse, httpapi.Audit("curso.atualizado")))
	r.Handle("DELETE", "/cursos/:cursoId", httpapi.Admin(msgAdminOnly, h.deleteCourse, httpapi.Audit("curso.removido")))

	r.Handle("POST", "/turmas", httpapi.Admin(msgAdminOnly, h.createCohort, httpapi.Audit("turma.criada")))
	r.Handle("GET", "/turmas", h.listCohorts)
	r.Handle("PUT", "/turmas/:turmaId", httpapi.Admin(msgAdminOnly, h.updateCohort, httpapi.Audit("turma.atualizada")))
	r.Handle("DELETE", "/turmas/:turmaId", httpapi.Admin(msgAdminOnly, h.deleteCohort, httpapi.Audit("turma.removida")))

	r.Handle("GET", "/admin/students", httpapi.Admin(msgAdminOnly, h.listEnrollments))
	r.Handle("POST", "/admin/students", httpapi.Admin(msgAdminOnly, h.enroll, httpapi.Audit("matricula.provisionado")))
	r.Handle("DELETE", "/admin/students", httpapi.Admin(msgAdminOnly, h.unenroll, httpapi.Audit("matricula.removido")))

	r.Handle("POST", "/programas", httpapi.Admin(msgAdminOnly, h.createProgram, httpapi.Audit("programa.criado")))
	r.Handle("GET", "/programas", h.listPrograms)
	r.Handle("PUT", "/programas/:programaId", httpapi.Admin(msgAdminOnly, h.updateProgram, httpapi.Audit("programa.atualizado")))
	r.Handle("DELETE", "/programas/:programaId", httpapi.Admin(msgAdminOnly, h.deleteProgram, httpapi.Audit("programa.removido")))
}

func (h handlers) createCourse(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in CourseInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	c, err := h.svc.CreateCourse(ctx, in)
	return httpapi.Reply(201, c, err)
}

func (h handlers) listCourses(ctx context.Context, _ *httpapi.Request) (httpapi.Response, error) {
	courses, err := h.svc.ListCourses(ctx)
	return httpapi.Reply(200, courses, err)
}

func (h handlers) updateCourse(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in CourseInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Curso atualizado.", h.svc.UpdateCourse(ctx, r.Param("cursoId"), in))
}

func (h handlers) deleteCourse(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("Curso excluído.", h.svc.DeleteCourse(ctx, r.Param("cursoId")))
}

func (h handlers) createCohort(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in CohortInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	c, err := h.svc.CreateCohort(ctx, in)
	return httpapi.Reply(201, c, err)
}

func (h handlers) listCohorts(ctx context.Context, _ *httpapi.Request) (httpapi.Response, error) {
	cohorts, err := h.svc.ListCohorts(ctx)
	return httpapi.Reply(200, cohorts, err)
}

func (h handlers) updateCohort(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in CohortInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Turma atualizada.", h.svc.UpdateCohort(ctx, r.Param("turmaId"), in))
}

func (h handlers) deleteCohort(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("Turma excluída.", h.svc.DeleteCohort(ctx, r.Param("turmaId")))
}

func (h handlers) listEnrollments(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	enrollments, err := h.svc.ListEnrollments(ctx, r.Query("turmaId"))
	return httpapi.Reply(200, enrollments, err)
}

func (h handlers) enroll(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in EnrollmentRequest
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	result, err := h.svc.Enroll(ctx, in)
	return httpapi.Reply(200, result, err)
}

func (h handlers) unenroll(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in EnrollmentRequest
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	result, err := h.svc.Unenroll(ctx, in)
	return httpapi.Reply(200, result, err)
}

func (h handlers) createProgram(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ProgramInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	p, err := h.svc.CreateProgram(ctx, in)
	return httpapi.Reply(201, p, err)
}

func (h handlers) listPrograms(ctx context.Context, _ *httpapi.Request) (httpapi.Response, error) {
	programs, err := h.svc.ListPrograms(ctx)
	return httpapi.Reply(200, programs, err)
}

func (h handlers) updateProgram(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ProgramInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Programa atualizado.", h.svc.UpdateProgram(ctx, r.Param("programaId"), in))
}

func (h handlers) deleteProgram(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("Programa excluído.", h.svc.DeleteProgram(ctx, r.Param("programaId")))
}
