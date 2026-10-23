// Package academic owns courses (cursos), cohorts (turmas), their PFC program
// and the RGM allowlist (matrículas). A cohort is a registered course + shift
// + term.
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

func courseName(s string) string { return strings.Join(strings.Fields(s), " ") }

func (in CourseInput) normalized() CourseInput { return CourseInput{Name: courseName(in.Name)} }

func (in CourseInput) validate() string {
	switch {
	case in.Name == "":
		return msgCourseNameRequired
	case utf8.RuneCountInString(in.Name) > maxCourseNameLen:
		return msgCourseNameTooLong
	}
	return ""
}

// Cohort.Course is the course name, read from the course on every listing so
// a rename never leaves cohorts behind.
type Cohort struct {
	ID       string `json:"id"`
	CourseID string `json:"graduacaoId,omitempty"`
	Course   string `json:"nome"`
	Shift    string `json:"turno"`
	Term     string `json:"periodo"`
}

type CohortInput struct {
	Course string `json:"nome"`
	Shift  string `json:"turno"`
	Term   string `json:"periodo"`
}

func (in CohortInput) normalized() CohortInput {
	return CohortInput{
		Course: courseName(in.Course),
		Shift:  strings.TrimSpace(in.Shift),
		Term:   normalizeTerm(in.Term),
	}
}

// The term is part of the cohort's uniqueness lock, so it has one spelling:
// "2026/2" and "2026-2" become "2026.2" and anything else is refused.
var termFormat = regexp.MustCompile(`^(\d{4})\s*[./-]\s*([12])$`)

func normalizeTerm(s string) string {
	s = strings.TrimSpace(s)
	if m := termFormat.FindStringSubmatch(s); m != nil {
		return m[1] + "." + m[2]
	}
	return s
}

func (in CohortInput) validate() string {
	switch {
	case in.Course == "" || in.Shift == "" || in.Term == "":
		return msgCohortFieldsRequired
	case !shifts[in.Shift]:
		return msgInvalidShift
	case !termFormat.MatchString(in.Term):
		return msgInvalidTerm
	}
	return ""
}

func (c Cohort) sameAs(courseID string, in CohortInput) bool {
	return c.CourseID == courseID && c.Shift == in.Shift && c.Term == in.Term
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
	msgAdminOnly            = "Acesso restrito a administradores"
	msgCohortFieldsRequired = "Nome, turno e período são obrigatórios"
	msgInvalidCourse        = "Curso inválido. Escolha um dos cursos disponíveis"
	msgCourseNameRequired   = "Nome do curso é obrigatório"
	msgCourseNameTooLong    = "Nome do curso longo demais"
	msgCourseDuplicate      = "Já existe um curso com esse nome"
	msgCourseChanged        = "O curso mudou enquanto você editava. Atualize a página e tente de novo"
	msgCourseUnknown        = "Curso não encontrado"
	msgCourseHasCohorts     = "O curso tem turmas e não pode ser removido"
	msgInvalidShift         = "Turno inválido. Escolha manhã ou noite"
	msgInvalidTerm          = "Período inválido. Use ano e semestre, como 2026.2"
	msgCohortDuplicate      = "Já existe uma turma com esse curso, turno e período"
	msgCohortChanged        = "A turma mudou enquanto você editava. Atualize a página e tente de novo"
	msgCohortNotFound       = "Turma não encontrada"
	msgCohortHasProgram     = "A turma tem programa de PFC e não pode ser removida"
	msgCohortIDRequired     = "Escolha a turma"
	msgCourseNotFound       = "Curso não encontrado"
	msgProgramDuplicate     = "Esta turma já tem um programa de PFC"
	msgProgramChanged       = "O programa mudou enquanto você editava. Atualize a página e tente de novo"
	msgProgramNotFound      = "Programa não encontrado"
	msgProgramHasProjects   = "O programa tem PFCs e não pode ser removido"
	msgEnrollCohortRequired = "Escolha a turma"
	msgRGMRequired          = "Informe ao menos um RGM"
	msgRGMDigitsOnly        = "RGM deve conter apenas números"
)
