package academic

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

// enrollmentBatch matches DynamoDB's BatchWriteItem limit; failures are
// reported per batch.
const enrollmentBatch = 25

type Service struct {
	store    Store
	projects Projects
	newID    func() string
}

func NewService(store Store, projects Projects) *Service {
	return &Service{store: store, projects: projects, newID: uuid.NewString}
}

func courseResource(id string) *audit.Resource  { return &audit.Resource{Type: "curso", ID: id} }
func cohortResource(id string) *audit.Resource  { return &audit.Resource{Type: "turma", ID: id} }
func programResource(id string) *audit.Resource { return &audit.Resource{Type: "programa", ID: id} }

// registeredCourse resolves the course a cohort names, ignoring case and
// repeated spaces; a name that is not a registered course is a validation
// error.
func (s *Service) registeredCourse(ctx context.Context, name string) (Course, error) {
	course, found, err := s.store.CourseByName(ctx, name)
	if err != nil {
		return Course{}, apperr.Internal("Não foi possível validar o curso. Tente novamente em instantes.", err)
	}
	if !found {
		return Course{}, apperr.Invalid(msgInvalidCourse)
	}
	return course, nil
}

func (s *Service) CreateCourse(ctx context.Context, in CourseInput) (Course, error) {
	in = in.normalized()
	if msg := in.validate(); msg != "" {
		return Course{}, apperr.Invalid(msg)
	}
	c := Course{ID: s.newID(), Name: in.Name}
	err := s.store.CreateCourse(ctx, c)
	if errors.Is(err, errDuplicate) {
		audit.Record(ctx, audit.Event{Action: "curso.criado", Result: audit.Failure, Reason: "curso já existe", Details: map[string]any{"nome": c.Name}})
		return Course{}, apperr.Conflict(msgCourseDuplicate)
	}
	if err != nil {
		return Course{}, apperr.Internal("Não foi possível criar o curso. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: "curso.criado", Result: audit.Success, Resource: courseResource(c.ID), Details: map[string]any{"nome": c.Name}})
	return c, nil
}

func (s *Service) ListCourses(ctx context.Context) ([]Course, error) {
	courses, err := s.store.ListCourses(ctx)
	if err != nil {
		return nil, apperr.Internal("Não foi possível carregar os cursos. Tente novamente em instantes.", err)
	}
	slices.SortStableFunc(courses, func(a, b Course) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return courses, nil
}

// UpdateCourse renames the course; cohorts point to it by id and show the new
// name on the next listing.
func (s *Service) UpdateCourse(ctx context.Context, id string, in CourseInput) error {
	in = in.normalized()
	if msg := in.validate(); msg != "" {
		return apperr.Invalid(msg)
	}
	current, found, err := s.store.GetCourse(ctx, id)
	if err != nil {
		return apperr.Internal("Não foi possível atualizar o curso. Tente novamente em instantes.", err)
	}
	if !found {
		return apperr.NotFound(msgCourseUnknown)
	}
	next := Course{ID: id, Name: in.Name}
	if current.Name != next.Name {
		switch err := s.store.UpdateCourse(ctx, current, next); {
		case errors.Is(err, errDuplicate):
			audit.Record(ctx, audit.Event{Action: "curso.atualizado", Result: audit.Failure, Reason: "curso já existe",
				Resource: courseResource(id), Details: map[string]any{"nome": next.Name}})
			return apperr.Conflict(msgCourseDuplicate)
		case errors.Is(err, errStale):
			return apperr.Conflict(msgCourseChanged)
		case err != nil:
			return apperr.Internal("Não foi possível atualizar o curso. Tente novamente em instantes.", err)
		}
	}
	audit.Record(ctx, audit.Event{Action: "curso.atualizado", Result: audit.Success, Resource: courseResource(id),
		Details: map[string]any{"nome": next.Name, "nomeAnterior": current.Name}})
	return nil
}

func (s *Service) DeleteCourse(ctx context.Context, id string) error {
	c, found, err := s.store.GetCourse(ctx, id)
	if err != nil {
		return apperr.Internal("Não foi possível excluir o curso. Tente novamente em instantes.", err)
	}
	if !found {
		return apperr.NotFound(msgCourseUnknown)
	}
	switch err := s.store.DeleteCourse(ctx, c); {
	case errors.Is(err, errInUse):
		audit.Record(ctx, audit.Event{Action: "curso.removido", Result: audit.Failure, Reason: "curso possui turmas", Resource: courseResource(id)})
		return apperr.Conflict(msgCourseHasCohorts)
	case errors.Is(err, errNotFound):
		return apperr.NotFound(msgCourseUnknown)
	case errors.Is(err, errStale):
		return apperr.Conflict(msgCourseChanged)
	case err != nil:
		return apperr.Internal("Não foi possível excluir o curso. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: "curso.removido", Result: audit.Success, Resource: courseResource(id)})
	return nil
}

func (s *Service) CreateCohort(ctx context.Context, in CohortInput) (Cohort, error) {
	in = in.normalized()
	if msg := in.validate(); msg != "" {
		return Cohort{}, apperr.Invalid(msg)
	}
	course, err := s.registeredCourse(ctx, in.Course)
	if err != nil {
		return Cohort{}, err
	}
	c := Cohort{ID: s.newID(), CourseID: course.ID, Course: course.Name, Shift: in.Shift, Term: in.Term}
	err = s.store.CreateCohort(ctx, c)
	if errors.Is(err, errNoCourse) {
		return Cohort{}, apperr.Invalid(msgInvalidCourse)
	}
	if errors.Is(err, errDuplicate) {
		audit.Record(ctx, audit.Event{Action: "turma.criada", Result: audit.Failure, Reason: "turma já existe", Details: c.details()})
		return Cohort{}, apperr.Conflict(msgCohortDuplicate)
	}
	if err != nil {
		return Cohort{}, apperr.Internal("Não foi possível criar a turma. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: "turma.criada", Result: audit.Success, Resource: cohortResource(c.ID), Details: c.details()})
	return c, nil
}

// ListCohorts takes the course name from the course itself. A cohort whose
// course is gone keeps the name it was stored with.
func (s *Service) ListCohorts(ctx context.Context) ([]Cohort, error) {
	cohorts, err := s.store.ListCohorts(ctx)
	if err != nil {
		return nil, apperr.Internal("Não foi possível carregar as turmas. Tente novamente em instantes.", err)
	}
	courses, err := s.store.ListCourses(ctx)
	if err != nil {
		return nil, apperr.Internal("Não foi possível carregar as turmas. Tente novamente em instantes.", err)
	}
	names := make(map[string]string, len(courses))
	for _, c := range courses {
		names[c.ID] = c.Name
	}
	for i, c := range cohorts {
		if name, ok := names[c.CourseID]; ok {
			cohorts[i].Course = name
		}
	}
	return cohorts, nil
}

func (s *Service) UpdateCohort(ctx context.Context, id string, in CohortInput) error {
	in = in.normalized()
	if msg := in.validate(); msg != "" {
		return apperr.Invalid(msg)
	}
	current, found, err := s.store.GetCohort(ctx, id)
	if err != nil {
		return apperr.Internal("Não foi possível atualizar a turma. Tente novamente em instantes.", err)
	}
	if !found {
		return apperr.NotFound(msgCohortNotFound)
	}
	course, err := s.registeredCourse(ctx, in.Course)
	if err != nil {
		return err
	}
	next := Cohort{ID: id, CourseID: course.ID, Course: course.Name, Shift: in.Shift, Term: in.Term}
	if !current.sameAs(course.ID, in) {
		switch err := s.store.UpdateCohort(ctx, current, next); {
		case errors.Is(err, errNoCourse):
			return apperr.Invalid(msgInvalidCourse)
		case errors.Is(err, errDuplicate):
			audit.Record(ctx, audit.Event{Action: "turma.atualizada", Result: audit.Failure, Reason: "turma já existe",
				Resource: cohortResource(id), Details: next.details()})
			return apperr.Conflict(msgCohortDuplicate)
		case errors.Is(err, errStale):
			return apperr.Conflict(msgCohortChanged)
		case err != nil:
			return apperr.Internal("Não foi possível atualizar a turma. Tente novamente em instantes.", err)
		}
	}
	audit.Record(ctx, audit.Event{Action: "turma.atualizada", Result: audit.Success, Resource: cohortResource(id), Details: next.details()})
	return nil
}

func (s *Service) DeleteCohort(ctx context.Context, id string) error {
	c, found, err := s.store.GetCohort(ctx, id)
	if err != nil {
		return apperr.Internal("Não foi possível excluir a turma. Tente novamente em instantes.", err)
	}
	if !found {
		return apperr.NotFound(msgCohortNotFound)
	}
	switch err := s.store.DeleteCohort(ctx, c); {
	case errors.Is(err, errInUse):
		audit.Record(ctx, audit.Event{Action: "turma.removida", Result: audit.Failure, Reason: "turma possui programa vinculado", Resource: cohortResource(id)})
		return apperr.Conflict(msgCohortHasProgram)
	case errors.Is(err, errNotFound):
		return apperr.NotFound(msgCohortNotFound)
	case errors.Is(err, errStale):
		return apperr.Conflict(msgCohortChanged)
	case err != nil:
		return apperr.Internal("Não foi possível excluir a turma. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: "turma.removida", Result: audit.Success, Resource: cohortResource(id)})
	return nil
}

func (s *Service) CreateProgram(ctx context.Context, in ProgramInput) (Program, error) {
	if in.CohortID == "" {
		return Program{}, apperr.Invalid(msgCohortIDRequired)
	}
	p := Program{ID: s.newID(), CohortID: in.CohortID}
	switch err := s.store.CreateProgram(ctx, p); {
	case errors.Is(err, errNoCohort):
		return Program{}, apperr.NotFound(msgCourseNotFound)
	case errors.Is(err, errDuplicate):
		audit.Record(ctx, audit.Event{Action: "programa.criado", Result: audit.Failure, Reason: "programa já existe para este curso",
			Details: map[string]any{"cursoId": in.CohortID}})
		return Program{}, apperr.Conflict(msgProgramDuplicate)
	case err != nil:
		return Program{}, apperr.Internal("Não foi possível criar o programa. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: "programa.criado", Result: audit.Success, Resource: programResource(p.ID),
		Details: map[string]any{"cursoId": p.CohortID}})
	return p, nil
}

func (s *Service) ListPrograms(ctx context.Context) ([]Program, error) {
	programs, err := s.store.ListPrograms(ctx)
	if err != nil {
		return nil, apperr.Internal("Não foi possível carregar os programas. Tente novamente em instantes.", err)
	}
	return programs, nil
}

func (s *Service) UpdateProgram(ctx context.Context, id string, in ProgramInput) error {
	if in.CohortID == "" {
		return apperr.Invalid(msgCohortIDRequired)
	}
	current, found, err := s.store.GetProgram(ctx, id)
	if err != nil {
		return apperr.Internal("Não foi possível atualizar o programa. Tente novamente em instantes.", err)
	}
	if !found {
		return apperr.NotFound(msgProgramNotFound)
	}
	if current.CohortID != in.CohortID {
		switch err := s.store.MoveProgram(ctx, current, in.CohortID); {
		case errors.Is(err, errNoCohort):
			return apperr.NotFound(msgCourseNotFound)
		case errors.Is(err, errDuplicate):
			audit.Record(ctx, audit.Event{Action: "programa.atualizado", Result: audit.Failure, Reason: "programa já existe para este curso",
				Resource: programResource(id), Details: map[string]any{"cursoId": in.CohortID}})
			return apperr.Conflict(msgProgramDuplicate)
		case errors.Is(err, errStale):
			return apperr.Conflict(msgProgramChanged)
		case err != nil:
			return apperr.Internal("Não foi possível atualizar o programa. Tente novamente em instantes.", err)
		}
	}
	audit.Record(ctx, audit.Event{Action: "programa.atualizado", Result: audit.Success, Resource: programResource(id),
		Details: map[string]any{"cursoId": in.CohortID}})
	return nil
}

// DeleteProgram checks for projects before the transaction: a GSI cannot be
// part of a condition, so a project created at the same instant slips
// through. Accepted: the operation is rare and admin-only.
func (s *Service) DeleteProgram(ctx context.Context, id string) error {
	p, found, err := s.store.GetProgram(ctx, id)
	if err != nil {
		return apperr.Internal("Não foi possível excluir o programa. Tente novamente em instantes.", err)
	}
	if !found {
		return apperr.NotFound(msgProgramNotFound)
	}
	hasProjects, err := s.projects.ProgramHasProjects(ctx, id)
	if err != nil {
		return apperr.Internal("Não foi possível excluir o programa. Tente novamente em instantes.", err)
	}
	if hasProjects {
		audit.Record(ctx, audit.Event{Action: "programa.removido", Result: audit.Failure, Reason: "programa possui projetos vinculados", Resource: programResource(id)})
		return apperr.Conflict(msgProgramHasProjects)
	}
	switch err := s.store.DeleteProgram(ctx, p); {
	case errors.Is(err, errNotFound):
		return apperr.NotFound(msgProgramNotFound)
	case errors.Is(err, errStale):
		return apperr.Conflict(msgProgramChanged)
	case err != nil:
		return apperr.Internal("Não foi possível excluir o programa. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: "programa.removido", Result: audit.Success, Resource: programResource(id)})
	return nil
}

func (s *Service) ListEnrollments(ctx context.Context) ([]Enrollment, error) {
	enrollments, err := s.store.ListEnrollments(ctx)
	if err != nil {
		return nil, apperr.Internal("Não foi possível carregar as matrículas. Tente novamente em instantes.", err)
	}
	return enrollments, nil
}

func (s *Service) Enroll(ctx context.Context, in EnrollmentRequest) (EnrollmentResult, error) {
	cohortID := strings.TrimSpace(in.CohortID)
	if cohortID == "" {
		return EnrollmentResult{}, apperr.Invalid(msgEnrollCohortRequired)
	}
	exists, err := s.store.CohortExists(ctx, cohortID)
	if err != nil {
		return EnrollmentResult{}, apperr.Internal("Não foi possível validar a turma. Tente novamente em instantes.", err)
	}
	if !exists {
		return EnrollmentResult{}, apperr.NotFound(msgCohortNotFound)
	}
	received := distinctRGMs(in.RGMs)
	if len(received) == 0 {
		return EnrollmentResult{}, apperr.Invalid(msgRGMRequired)
	}
	valid, failures := splitByFormat(received)
	writeFailures := inBatches(valid, func(batch []string) error { return s.store.PutEnrollments(ctx, batch, cohortID) })
	result := EnrollmentResult{Processed: len(valid) - len(writeFailures), Failures: append(failures, writeFailures...)}

	s.recordBatch(ctx, "matricula.provisionado", len(received), result,
		map[string]any{"rgms": received, "turmaId": cohortID, "processados": result.Processed})
	return result, nil
}

func (s *Service) Unenroll(ctx context.Context, in EnrollmentRequest) (EnrollmentResult, error) {
	rgms := distinctRGMs(in.RGMs)
	if len(rgms) == 0 {
		return EnrollmentResult{}, apperr.Invalid(msgRGMRequired)
	}
	failures := inBatches(rgms, func(batch []string) error { return s.store.DeleteEnrollments(ctx, batch) })
	result := EnrollmentResult{Processed: len(rgms) - len(failures), Failures: failures}

	s.recordBatch(ctx, "matricula.removido", len(rgms), result,
		map[string]any{"rgms": rgms, "processados": result.Processed})
	return result, nil
}

func (s *Service) recordBatch(ctx context.Context, action string, total int, result EnrollmentResult, details map[string]any) {
	ev := audit.Event{Action: action, Result: audit.Success, Details: details}
	if len(result.Failures) > 0 {
		ev.Result = audit.Failure
		ev.Reason = fmt.Sprintf("%d de %d RGMs falharam", len(result.Failures), total)
	}
	audit.Record(ctx, ev)
}

func inBatches(rgms []string, write func([]string) error) []EnrollmentFailure {
	var failures []EnrollmentFailure
	for start := 0; start < len(rgms); start += enrollmentBatch {
		batch := rgms[start:min(start+enrollmentBatch, len(rgms))]
		if err := write(batch); err != nil {
			for _, rgm := range batch {
				failures = append(failures, EnrollmentFailure{RGM: rgm, Error: err.Error()})
			}
		}
	}
	return failures
}

// Cohort takes the course name from the course, like ListCohorts.
func (s *Service) Cohort(ctx context.Context, id string) (Cohort, bool, error) {
	if id == "" {
		return Cohort{}, false, nil
	}
	c, found, err := s.store.GetCohort(ctx, id)
	if err != nil || !found || c.CourseID == "" {
		return c, found, err
	}
	course, ok, err := s.store.GetCourse(ctx, c.CourseID)
	if err != nil {
		return Cohort{}, false, err
	}
	if ok {
		c.Course = course.Name
	}
	return c, true, nil
}

func (s *Service) CohortExists(ctx context.Context, id string) (bool, error) {
	return s.store.CohortExists(ctx, id)
}

func (s *Service) CohortOfProgram(ctx context.Context, programID string) (string, bool, error) {
	p, found, err := s.store.GetProgram(ctx, programID)
	return p.CohortID, found, err
}

func (s *Service) ProgramExists(ctx context.Context, id string) (bool, error) {
	return s.store.ProgramExists(ctx, id)
}

func (s *Service) ProgramOfCohort(ctx context.Context, cohortID string) (string, bool, error) {
	if cohortID == "" {
		return "", false, nil
	}
	return s.store.ProgramOfCohort(ctx, cohortID)
}

func (s *Service) IsEnrolled(ctx context.Context, rgm string) (bool, error) {
	return s.store.EnrollmentExists(ctx, rgm)
}

func (s *Service) CohortOf(ctx context.Context, rgm string) (string, error) {
	if rgm == "" {
		return "", nil
	}
	return s.store.CohortOfStudent(ctx, rgm)
}

func (s *Service) CohortsOf(ctx context.Context, rgms []string) (map[string]string, error) {
	return s.store.CohortsOfStudents(ctx, rgms)
}
