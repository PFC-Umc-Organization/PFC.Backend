// Package deliveries owns activities, the submission form of each one
// (fields) and the groups' submissions. Activities are global on purpose:
// every cohort and shift has the same PFC deliverables.
package deliveries

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"
)

type FieldType string

const (
	FieldFile     FieldType = "ARQUIVO"
	FieldText     FieldType = "TEXTO"
	FieldLongText FieldType = "TEXTO_LONGO"
	FieldLink     FieldType = "LINK"
)

const (
	maxTitleLen       = 150
	maxDescriptionLen = 5000
	maxLabelLen       = 100
	maxTextLen        = 300
	maxLongTextLen    = 5000
	maxFileNameLen    = 400
	maxFields         = 20
)

// A file field's answer is the key of the uploaded file (see files.go), or
// a bare file name in submissions made before storage existed.
type Field struct {
	ID       string    `json:"id"`
	Label    string    `json:"rotulo"`
	Type     FieldType `json:"tipo"`
	Required bool      `json:"obrigatorio"`
}

type FieldInput struct {
	Label    string    `json:"rotulo"`
	Type     FieldType `json:"tipo"`
	Required bool      `json:"obrigatorio"`
}

// Deadline comes from <input type="datetime-local">, with no time zone, and
// is read as Brasília time.
type Activity struct {
	ID          string  `json:"id"`
	Title       string  `json:"titulo"`
	Description string  `json:"descricao"`
	Deadline    string  `json:"prazo"`
	PublishedAt string  `json:"publicadaEm"`
	Fields      []Field `json:"campos"`
}

// ActivityInput.Fields is only read on creation; empty means the default
// file field.
type ActivityInput struct {
	Title       string       `json:"titulo"`
	Description string       `json:"descricao"`
	Deadline    string       `json:"prazo"`
	Fields      []FieldInput `json:"campos"`
}

// Answers are keyed by field id. SubmittedAt is the first submission, the
// one the deadline is checked against; editing sets UpdatedAt.
type Submission struct {
	ActivityID  string            `json:"atividadeId"`
	ProjectID   string            `json:"projetoId"`
	SubmittedAt string            `json:"entregueEm"`
	SubmittedBy string            `json:"entreguePor"`
	UpdatedAt   string            `json:"atualizadaEm,omitempty"`
	UpdatedBy   string            `json:"atualizadaPor,omitempty"`
	Answers     map[string]string `json:"respostas"`
}

type SubmissionInput struct {
	Answers map[string]string `json:"respostas"`
}

var deadlineLayouts = []string{"2006-01-02T15:04", "2006-01-02T15:04:05", time.RFC3339}

var brasilia = mustLoadLocation("America/Sao_Paulo")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

func deadlineOf(s string) (time.Time, bool) {
	for _, l := range deadlineLayouts {
		if t, err := time.ParseInLocation(l, s, brasilia); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func validDeadline(s string) bool {
	_, ok := deadlineOf(s)
	return ok
}

// closed tells whether a group without a submission can no longer submit.
func (a Activity) closed(now time.Time) bool {
	deadline, ok := deadlineOf(a.Deadline)
	return ok && now.After(deadline)
}

func (in ActivityInput) normalized() ActivityInput {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.Deadline = strings.TrimSpace(in.Deadline)
	return in
}

func (in ActivityInput) validate() string {
	switch {
	case in.Title == "":
		return "Informe o título."
	case utf8.RuneCountInString(in.Title) > maxTitleLen:
		return "O título excede o tamanho máximo permitido."
	case utf8.RuneCountInString(in.Description) > maxDescriptionLen:
		return "A descrição excede o tamanho máximo permitido."
	case !validDeadline(in.Deadline):
		return "Prazo inválido. Informe a data e a hora limite."
	}
	return ""
}

func (in FieldInput) normalized() (FieldInput, string) {
	in.Label = strings.TrimSpace(in.Label)
	switch {
	case in.Label == "":
		return in, "Informe o nome do campo."
	case utf8.RuneCountInString(in.Label) > maxLabelLen:
		return in, "O nome do campo excede o tamanho máximo permitido."
	}
	switch in.Type {
	case FieldFile, FieldText, FieldLongText, FieldLink:
		return in, ""
	}
	return in, "Tipo de campo inválido."
}

var defaultFields = []FieldInput{{Label: "Arquivo da entrega", Type: FieldFile, Required: true}}

func normalizedFields(fields []FieldInput) ([]FieldInput, string) {
	if len(fields) == 0 {
		return defaultFields, ""
	}
	if len(fields) > maxFields {
		return nil, "A atividade excede o número máximo de campos."
	}
	out := make([]FieldInput, 0, len(fields))
	for _, f := range fields {
		n, msg := f.normalized()
		if msg != "" {
			return nil, msg
		}
		out = append(out, n)
	}
	return out, ""
}

func validAnswers(fields []Field, answers map[string]string) (map[string]string, string) {
	known := make(map[string]bool, len(fields))
	for _, f := range fields {
		known[f.ID] = true
	}
	for id := range answers {
		if !known[id] {
			return nil, "A entrega contém um campo que não pertence à atividade. Atualize a página e tente novamente."
		}
	}

	out := make(map[string]string, len(fields))
	for _, f := range fields {
		v := strings.TrimSpace(answers[f.ID])
		if v == "" {
			if f.Required {
				return nil, fmt.Sprintf("O campo %q é obrigatório.", f.Label)
			}
			continue
		}
		limit := maxTextLen
		switch f.Type {
		case FieldLongText:
			limit = maxLongTextLen
		case FieldFile:
			limit = maxFileNameLen
		case FieldLink:
			if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
				return nil, fmt.Sprintf("O campo %q precisa de um link iniciado por http:// ou https://.", f.Label)
			}
		}
		if utf8.RuneCountInString(v) > limit {
			return nil, fmt.Sprintf("O campo %q excede o tamanho máximo permitido.", f.Label)
		}
		out[f.ID] = v
	}
	return out, ""
}

const (
	msgStaffOnly          = "Acesso restrito a orientadores e à coordenação."
	msgMembersAndStaff    = "Acesso restrito aos integrantes, ao orientador do projeto e à coordenação."
	msgReviewersOnly      = "Apenas a coordenação e o orientador do projeto podem devolver uma entrega."
	msgAdminOnly          = "Acesso restrito à coordenação."
	msgMembersOnly        = "Apenas integrantes do projeto podem enviar entregas."
	msgActivityNotFound   = "Atividade não encontrada."
	msgFieldNotFound      = "Campo não encontrado."
	msgLastField          = "A atividade precisa de ao menos um campo de entrega."
	msgFieldLimit         = "A atividade já atingiu o número máximo de campos."
	msgSubmissionNotFound = "Entrega não encontrada."
	msgProjectNotFound    = "Projeto não encontrado."
	msgDeadlinePassed     = "O prazo desta atividade está encerrado e ela não recebe mais entregas."
	msgGroupFailed        = "O grupo foi reprovado por atingir o limite de penalidades, e o envio de entregas está encerrado."
	msgPenaltyReviewers   = "Apenas a coordenação e o orientador do projeto podem administrar penalidades."
	msgPenaltyNotFound    = "Penalidade não encontrada."
	msgAlreadyWaived      = "Esta penalidade já foi perdoada."
	msgReasonRequired     = "Informe o motivo."
	msgReasonTooLong      = "O motivo excede o tamanho máximo permitido."
)
