// Package deliveries owns activities, the submission form of each one
// (fields) and the groups' submissions. Activities are global on purpose:
// every cohort and shift has the same PFC deliverables.
package deliveries

import (
	"fmt"
	"strings"
	"time"
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
	maxFileNameLen    = 255
	maxFields         = 20
)

// A file field stores only the file name: there is no file storage yet.
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

// Deadline comes from <input type="datetime-local">, with no time zone.
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

// Answers are keyed by field id.
type Submission struct {
	ActivityID  string            `json:"atividadeId"`
	ProjectID   string            `json:"projetoId"`
	SubmittedAt string            `json:"entregueEm"`
	SubmittedBy string            `json:"entreguePor"`
	Answers     map[string]string `json:"respostas"`
}

type SubmissionInput struct {
	Answers map[string]string `json:"respostas"`
}

var deadlineLayouts = []string{"2006-01-02T15:04", "2006-01-02T15:04:05", time.RFC3339}

func validDeadline(s string) bool {
	for _, l := range deadlineLayouts {
		if _, err := time.Parse(l, s); err == nil {
			return true
		}
	}
	return false
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
		return "título é obrigatório"
	case utf8.RuneCountInString(in.Title) > maxTitleLen:
		return "título longo demais"
	case utf8.RuneCountInString(in.Description) > maxDescriptionLen:
		return "descrição longa demais"
	case !validDeadline(in.Deadline):
		return "prazo inválido"
	}
	return ""
}

func (in FieldInput) normalized() (FieldInput, string) {
	in.Label = strings.TrimSpace(in.Label)
	switch {
	case in.Label == "":
		return in, "nome do campo é obrigatório"
	case utf8.RuneCountInString(in.Label) > maxLabelLen:
		return in, "nome do campo longo demais"
	}
	switch in.Type {
	case FieldFile, FieldText, FieldLongText, FieldLink:
		return in, ""
	}
	return in, "tipo de campo inválido"
}

var defaultFields = []FieldInput{{Label: "Arquivo da entrega", Type: FieldFile, Required: true}}

func normalizedFields(fields []FieldInput) ([]FieldInput, string) {
	if len(fields) == 0 {
		return defaultFields, ""
	}
	if len(fields) > maxFields {
		return nil, "campos demais na atividade"
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

// validAnswers checks answers against the fields: known ids only, required
// ones filled, size and format per type. Blank answers are dropped.
func validAnswers(fields []Field, answers map[string]string) (map[string]string, string) {
	known := make(map[string]bool, len(fields))
	for _, f := range fields {
		known[f.ID] = true
	}
	for id := range answers {
		if !known[id] {
			return nil, "a resposta tem um campo que não existe na atividade"
		}
	}

	out := make(map[string]string, len(fields))
	for _, f := range fields {
		v := strings.TrimSpace(answers[f.ID])
		if v == "" {
			if f.Required {
				return nil, fmt.Sprintf("o campo %q é obrigatório", f.Label)
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
				return nil, fmt.Sprintf("o campo %q precisa de um link http(s)", f.Label)
			}
		}
		if utf8.RuneCountInString(v) > limit {
			return nil, fmt.Sprintf("o campo %q passou do tamanho máximo", f.Label)
		}
		out[f.ID] = v
	}
	return out, ""
}

const (
	msgStaffOnly          = "acesso restrito à equipe acadêmica"
	msgMembersAndStaff    = "acesso restrito aos integrantes do projeto e à equipe acadêmica"
	msgMembersOnly        = "apenas integrantes do projeto podem entregar"
	msgActivityNotFound   = "atividade não encontrada"
	msgFieldNotFound      = "campo não encontrado"
	msgLastField          = "a atividade precisa de ao menos um campo de entrega"
	msgFieldLimit         = "limite de campos da atividade atingido"
	msgSubmissionNotFound = "entrega não encontrada"
	msgProjectNotFound    = "projeto não encontrado"
)
