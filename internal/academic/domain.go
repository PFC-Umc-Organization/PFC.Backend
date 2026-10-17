// Package academic owns courses (cursos), cohorts (turmas), their PFC program
// and the RGM allowlist (matrículas). A cohort is a course + shift + term and
// keeps the course name, which must belong to a registered course.
package academic

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Shifts mirror Turno in the frontend (curso.model.ts).
var shifts = map[string]bool{"MANHA": true, "NOITE": true}

const maxCourseNameLen = 100

type Course struct {
	ID   string `json:"id"`
	Name string `json:"nome"`
}

type CourseInput struct {
	Name string `json:"nome"`
}

func (in CourseInput) normalized() CourseInput {
	return CourseInput{Name: strings.Join(strings.Fields(in.Name), " ")}
}

func (in CourseInput) validate() string {
	switch {
	case in.Name == "":
		return msgCourseNameRequired
	case utf8.RuneCountInString(in.Name) > maxCourseNameLen:
		return msgCourseNameTooLong
	}
	return ""
}

type Cohort struct {
	ID     string `json:"id"`
	Course string `json:"nome"`
	Shift  string `json:"turno"`
	Term   string `json:"periodo"`
}

type CohortInput struct {
	Course string `json:"nome"`
	Shift  string `json:"turno"`
	Term   string `json:"periodo"`
}

func (in CohortInput) normalized() CohortInput {
	return CohortInput{
		Course: strings.TrimSpace(in.Course),
		Shift:  strings.TrimSpace(in.Shift),
		Term:   strings.TrimSpace(in.Term),
	}
}

func (in CohortInput) validate() string {
	switch {
	case in.Course == "" || in.Shift == "" || in.Term == "":
		return msgCohortFieldsRequired
	case !shifts[in.Shift]:
		return msgInvalidShift
	}
	return ""
}

func (c Cohort) sameAs(in CohortInput) bool {
	return c.Course == in.Course && c.Shift == in.Shift && c.Term == in.Term
}

func (c Cohort) details() map[string]any {
	return map[string]any{"nome": c.Course, "turno": c.Shift, "periodo": c.Term}
}

type Program struct {
	ID       string `json:"id"`
	CohortID string `json:"cursoId"`
}

type ProgramInput struct {
	CohortID string `json:"cursoId"`
}

const (
	EnrollmentActive   = "ATIVO"
	EnrollmentInactive = "INATIVO"
)

type Enrollment struct {
	RGM      string `json:"rgm"`
	Status   string `json:"status"`
	CohortID string `json:"turmaId,omitempty"`
}

// EnrollmentRequest.CohortID is only read when enrolling.
type EnrollmentRequest struct {
	RGMs     []string `json:"rgms"`
	CohortID string   `json:"turmaId"`
}

type EnrollmentFailure struct {
	RGM   string `json:"rgm"`
	Error string `json:"erro"`
}

type EnrollmentResult struct {
	Processed int                 `json:"processados"`
	Failures  []EnrollmentFailure `json:"falhas,omitempty"`
}

// distinctRGMs trims, drops blanks and duplicates: a blank RGM would write a
// bare "STUDENT#" item.
func distinctRGMs(rgms []string) []string {
	seen := make(map[string]bool, len(rgms))
	out := make([]string, 0, len(rgms))
	for _, rgm := range rgms {
		rgm = strings.TrimSpace(rgm)
		if rgm == "" || seen[rgm] {
			continue
		}
		seen[rgm] = true
		out = append(out, rgm)
	}
	return out
}

// The RGM is the local part of the student's e-mail, digits only.
var digitsOnly = regexp.MustCompile(`^\d+$`)

func splitByFormat(rgms []string) (valid []string, invalid []EnrollmentFailure) {
	for _, rgm := range rgms {
		if digitsOnly.MatchString(rgm) {
			valid = append(valid, rgm)
		} else {
			invalid = append(invalid, EnrollmentFailure{RGM: rgm, Error: msgRGMDigitsOnly})
		}
	}
	return valid, invalid
}

const (
	msgAdminOnly            = "acesso restrito a administradores"
	msgCohortFieldsRequired = "nome, turno e periodo são obrigatórios"
	msgInvalidCourse        = "curso inválido — escolha um dos cursos disponíveis"
	msgCourseNameRequired   = "nome do curso é obrigatório"
	msgCourseNameTooLong    = "nome do curso longo demais"
	msgCourseDuplicate      = "já existe um curso com esse nome"
	msgCourseChanged        = "o curso mudou enquanto você editava — atualize a página e tente de novo"
	msgCourseUnknown        = "curso não encontrado"
	msgCourseHasCohorts     = "curso possui turmas, não pode ser removido"
	msgCourseTooManyCohorts = "o curso tem turmas demais para ser renomeado de uma vez — remova turmas antigas primeiro"
	msgInvalidShift         = "turno inválido — escolha manhã ou noite"
	msgCohortDuplicate      = "já existe uma turma com esse curso, turno e período"
	msgCohortChanged        = "a turma mudou enquanto você editava — atualize a página e tente de novo"
	msgCohortNotFound       = "turma não encontrada"
	msgCohortHasProgram     = "turma possui programa vinculado, não pode ser removida"
	msgCohortIDRequired     = "cursoId é obrigatório"
	msgCourseNotFound       = "curso não encontrado"
	msgProgramDuplicate     = "este curso já possui um programa de PFC"
	msgProgramChanged       = "o programa mudou enquanto você editava — atualize a página e tente de novo"
	msgProgramNotFound      = "programa não encontrado"
	msgProgramHasProjects   = "programa possui projetos vinculados, não pode ser removido"
	msgEnrollCohortRequired = "turmaId é obrigatório"
	msgRGMRequired          = "informe ao menos um RGM"
	msgRGMDigitsOnly        = "RGM deve conter apenas números"
)
