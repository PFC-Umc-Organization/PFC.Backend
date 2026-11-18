package announcements

import (
	"strings"
	"unicode/utf8"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
)

// ---------------------------------------------------------------------------
// Aviso
// ---------------------------------------------------------------------------

const (
	maxTitleLen   = 150
	maxContentLen = 5000
)

// Modelo
type Announcement struct {
	ID         string `json:"id"`
	Title      string `json:"titulo"`
	Content    string `json:"conteudo"`
	CohortID   string `json:"turmaId,omitempty"`
	AuthorID   string `json:"autorId"`
	AuthorName string `json:"autorNome"`
	AuthorRole string `json:"autorPapel"`
	CreatedAt  string `json:"criadoEm"`
}

// Entrada
type AnnouncementInput struct {
	Title    string `json:"titulo"`
	Content  string `json:"conteudo"`
	CohortID string `json:"turmaId"`
}

// Normalização
func (in AnnouncementInput) normalized() AnnouncementInput {
	in.Title = strings.TrimSpace(in.Title)
	in.Content = strings.TrimSpace(in.Content)
	in.CohortID = strings.TrimSpace(in.CohortID)
	return in
}

// Validação
func (in AnnouncementInput) validate() string {
	switch {
	case in.Title == "":
		return msgTitleRequired
	case utf8.RuneCountInString(in.Title) > maxTitleLen:
		return msgTitleTooLong
	case in.Content == "":
		return msgContentRequired
	case utf8.RuneCountInString(in.Content) > maxContentLen:
		return msgContentTooLong
	}
	return ""
}

// ---------------------------------------------------------------------------
// Permissões
// ---------------------------------------------------------------------------

// Verificação
func (a Announcement) canRemove(who actor.Actor) bool {
	return who.HasRole(actor.RoleAdmin) || (who.Sub != "" && a.AuthorID == who.Sub)
}

// Verificação
func (a Announcement) visibleIn(cohorts map[string]bool) bool {
	return a.CohortID == "" || cohorts[a.CohortID]
}

const (
	msgStaffOnly       = "Acesso restrito a orientadores e à coordenação."
	msgTitleRequired   = "Informe o título."
	msgTitleTooLong    = "O título excede o tamanho máximo permitido."
	msgContentRequired = "Informe o conteúdo do aviso."
	msgContentTooLong  = "O conteúdo excede o tamanho máximo permitido."
	msgCohortNotFound  = "Turma não encontrada."
	msgAdvisorCohort   = "Orientadores só podem publicar avisos para turmas em que têm orientandos."
	msgRemoveOwn       = "Orientadores só podem excluir os avisos que publicaram."
	msgNotFound        = "Aviso não encontrado."
)
