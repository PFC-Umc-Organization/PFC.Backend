// Package groups owns PFC projects: members, advisor and the invitations
// students send to classmates.
package groups

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
)

const (
	maxNameLen        = 150
	maxDescriptionLen = 2000
	MaxMembers        = 10
)

// Members holds RGMs; the advisor is a Cognito sub.
type Project struct {
	ID          string   `json:"id"`
	Name        string   `json:"nome"`
	Description string   `json:"descricao"`
	ProgramID   string   `json:"programaId"`
	Members     []string `json:"integrantes"`
	AdvisorID   string   `json:"orientadorId,omitempty"`
}

// ProjectInput.Members is only read on POST /meu-pfc, where it lists the
// classmates to invite.
type ProjectInput struct {
	Name        string   `json:"nome"`
	Description string   `json:"descricao"`
	Members     []string `json:"integrantes"`
}

func (in ProjectInput) normalized() ProjectInput {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	return in
}

func (in ProjectInput) validate() string {
	switch {
	case in.Name == "":
		return msgNameRequired
	case utf8.RuneCountInString(in.Name) > maxNameLen:
		return msgNameTooLong
	case utf8.RuneCountInString(in.Description) > maxDescriptionLen:
		return msgDescriptionTooLong
	}
	return ""
}

type MemberInput struct {
	RGM string `json:"rgm"`
}

type AdvisorInput struct {
	AdvisorID string `json:"orientadorId"`
}

// Invitation is pending until the student accepts or declines it; only
// then the RGM joins Members.
type Invitation struct {
	ProjectID   string `json:"projetoId"`
	ProjectName string `json:"projetoNome"`
	RGM         string `json:"rgm"`
	InvitedBy   string `json:"convidadoPor"`
	CreatedAt   string `json:"criadoEm"`
}

func isStaff(a actor.Actor) bool { return a.HasRole(actor.RoleAdmin, actor.RoleAdvisor) }

// IsMember never holds for staff: admins and advisors follow a project
// without belonging to it.
func IsMember(a actor.Actor, p Project) bool {
	if isStaff(a) {
		return false
	}
	rgm := a.RGM()
	return rgm != "" && slices.Contains(p.Members, rgm)
}

func CanView(a actor.Actor, p Project) bool { return isStaff(a) || IsMember(a, p) }

// group puts the creator first, then the distinct non-blank classmates.
func group(creator string, classmates []string) []string {
	seen := map[string]bool{creator: true}
	out := []string{creator}
	for _, rgm := range classmates {
		rgm = strings.TrimSpace(rgm)
		if rgm == "" || seen[rgm] {
			continue
		}
		seen[rgm] = true
		out = append(out, rgm)
	}
	return out
}

const (
	msgAdminOnly            = "acesso restrito a administradores"
	msgStudentsOnly         = "esta rota é para alunos"
	msgStudentsOnlyCreate   = "esta rota é para alunos — a coordenação cria projetos pela gestão de PFC"
	msgNoRGM                = "não foi possível identificar o seu RGM"
	msgMembersAndStaffOnly  = "acesso restrito aos integrantes do projeto e à equipe acadêmica"
	msgMembersOnlyInvite    = "apenas integrantes do projeto podem convidar"
	msgMembersOnlyCancel    = "apenas integrantes do projeto podem cancelar convites"
	msgNameRequired         = "nome é obrigatório"
	msgNameTooLong          = "nome longo demais"
	msgDescriptionTooLong   = "descrição longa demais"
	msgGroupTooLarge        = "grupo grande demais"
	msgRGMRequired          = "rgm é obrigatório"
	msgAdvisorRequired      = "orientadorId é obrigatório"
	msgAdvisorNotFound      = "orientador não encontrado"
	msgProjectNotFound      = "projeto não encontrado"
	msgProgramNotFound      = "programa não encontrado"
	msgGroupChanged         = "o grupo mudou enquanto você editava — atualize a página e tente de novo"
	msgGroupFull            = "o grupo já está cheio"
	msgGroupFullWithPending = "o grupo já está cheio (contando os convites pendentes)"
	msgNotPreauthorized     = "RGM não está pré-autorizado e não tem conta de aluno"
	msgAlreadyInProject     = "você já faz parte de um projeto"
	msgNoCohort             = "o seu RGM não está pré-autorizado em nenhuma turma — fale com a coordenação"
	msgCohortWithoutProgram = "a sua turma ainda não tem PFC iniciado — fale com a coordenação"
	msgOutsideGroupCohort   = "você não está pré-autorizado na turma deste grupo — fale com a coordenação"
	msgInvitationNotFound   = "convite não encontrado"
	msgGroupGone            = "este grupo não existe mais"
	msgCannotJoin           = "não foi possível entrar: o grupo está cheio ou você já é integrante"
)

func msgAlreadyMember(rgm string) string  { return "o RGM " + rgm + " já é integrante do grupo" }
func msgNotMember(rgm string) string      { return "o RGM " + rgm + " não é integrante do grupo" }
func msgInOtherProject(rgm string) string { return "o RGM " + rgm + " já faz parte de outro projeto" }
func msgNotInCohort(rgm string) string {
	return "o RGM " + rgm + " não está pré-autorizado na sua turma"
}
func msgNotInGroupCohort(rgm string) string {
	return "o RGM " + rgm + " não está pré-autorizado na turma do grupo"
}
func msgAlreadyInvited(rgm string) string { return "o RGM " + rgm + " já tem um convite deste grupo" }
func msgInNamedProject(rgm, name string) string {
	return "o RGM " + rgm + " já faz parte do projeto \"" + name + "\""
}
