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
	msgAdminOnly            = "Acesso restrito à coordenação."
	msgCohortFieldsRequired = "Informe o curso, o turno e o período."
	msgInvalidCourse        = "Curso inválido. Escolha um dos cursos cadastrados."
	msgCourseNameRequired   = "Informe o nome do curso."
	msgCourseNameTooLong    = "O nome do curso excede o tamanho máximo permitido."
	msgCourseDuplicate      = "Já existe um curso com este nome."
	msgCourseChanged        = "O curso foi alterado por outra pessoa enquanto você editava. Atualize a página e tente novamente."
	msgCourseUnknown        = "Curso não encontrado."
	msgCourseHasCohorts     = "Não é possível excluir um curso com turmas cadastradas. Exclua as turmas primeiro."
	msgInvalidShift         = "Turno inválido. Escolha manhã ou noite."
	msgInvalidTerm          = "Período inválido. Informe ano e semestre, como 2026.2."
	msgCohortDuplicate      = "Já existe uma turma com este curso, turno e período."
	msgCohortChanged        = "A turma foi alterada por outra pessoa enquanto você editava. Atualize a página e tente novamente."
	msgCohortNotFound       = "Turma não encontrada."
	msgCohortHasProgram     = "Não é possível excluir uma turma com programa de PFC. Exclua o programa primeiro."
	msgCohortHasEnrollments = "Não é possível excluir uma turma com RGMs pré-autorizados. Remova as pré-autorizações da turma em Usuários antes de excluí-la."
	msgNotEnrolled          = "Este RGM não está pré-autorizado em nenhuma turma."
	msgCohortIDRequired     = "Selecione a turma."
	msgCourseNotFound       = "Curso não encontrado."
	msgProgramDuplicate     = "Esta turma já tem um programa de PFC."
	msgProgramChanged       = "O programa foi alterado por outra pessoa enquanto você editava. Atualize a página e tente novamente."
	msgProgramNotFound      = "Programa não encontrado."
	msgProgramHasProjects   = "Não é possível excluir um programa com PFCs cadastrados."
	msgEnrollCohortRequired = "Selecione a turma."
	msgRGMRequired          = "Informe ao menos um RGM."
	msgRGMDigitsOnly        = "O RGM deve conter apenas números."
)
