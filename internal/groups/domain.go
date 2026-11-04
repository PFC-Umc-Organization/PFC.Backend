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

type Project struct {
	ID          string   `json:"id"`
	Name        string   `json:"nome"`
	Description string   `json:"descricao"`
	ProgramID   string   `json:"programaId"`
	Members     []string `json:"integrantes"`
	AdvisorID   string   `json:"orientadorId,omitempty"`
}

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

type Invitation struct {
	ProjectID   string `json:"projetoId"`
	ProjectName string `json:"projetoNome"`
	RGM         string `json:"rgm"`
	Name        string `json:"nome,omitempty"`
	InvitedBy   string `json:"convidadoPor"`
	CreatedAt   string `json:"criadoEm"`
}

func isStaff(a actor.Actor) bool { return a.HasRole(actor.RoleAdmin, actor.RoleAdvisor) }

func IsMember(a actor.Actor, p Project) bool {
	if isStaff(a) {
		return false
	}
	rgm := a.RGM()
	return rgm != "" && slices.Contains(p.Members, rgm)
}

func Advises(a actor.Actor, p Project) bool {
	return a.HasRole(actor.RoleAdvisor) && a.Sub != "" && p.AdvisorID == a.Sub
}

func CanReview(a actor.Actor, p Project) bool { return a.HasRole(actor.RoleAdmin) || Advises(a, p) }

func CanView(a actor.Actor, p Project) bool { return CanReview(a, p) || IsMember(a, p) }

type Member struct {
	RGM  string  `json:"rgm"`
	Name *string `json:"nome"`
}

type Advisor struct {
	ID   string `json:"id"`
	Name string `json:"nome"`
}

type Cohort struct {
	ID     string `json:"id"`
	Course string `json:"nome"`
	Shift  string `json:"turno"`
	Term   string `json:"periodo"`
}

type Detail struct {
	Project Project  `json:"projeto"`
	Members []Member `json:"integrantes"`
	Advisor *Advisor `json:"orientador"`
	Cohort  *Cohort  `json:"turma"`
}

type MyPFC struct {
	Cohort    *Cohort `json:"turma"`
	ProgramID string  `json:"programaId,omitempty"`
	Group     *Detail `json:"grupo"`
}

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
	msgAdminOnly            = "Acesso restrito à coordenação."
	msgStudentsOnly         = "Esta funcionalidade é exclusiva para alunos."
	msgStudentsOnlyCreate   = "Esta funcionalidade é exclusiva para alunos. A coordenação cadastra projetos pela Gestão de PFC."
	msgNoRGM                = "Não foi possível identificar o seu RGM. Entre novamente e, se o problema persistir, procure a coordenação."
	msgMembersAndStaffOnly  = "Acesso restrito aos integrantes, ao orientador do projeto e à coordenação."
	msgStaffOnly            = "Acesso restrito a orientadores e à coordenação."
	msgMembersOnlyInvite    = "Apenas integrantes do projeto podem enviar convites."
	msgMembersOnlyCancel    = "Apenas integrantes do projeto podem cancelar convites."
	msgNameRequired         = "Informe o nome do projeto."
	msgNameTooLong          = "O nome excede o tamanho máximo permitido."
	msgDescriptionTooLong   = "A descrição excede o tamanho máximo permitido."
	msgGroupTooLarge        = "O grupo excede o limite de integrantes."
	msgRGMRequired          = "Informe o RGM."
	msgAdvisorRequired      = "Selecione o orientador."
	msgAdvisorNotFound      = "Orientador não encontrado."
	msgProjectNotFound      = "Projeto não encontrado."
	msgProgramNotFound      = "Programa não encontrado."
	msgGroupChanged         = "O grupo foi alterado por outra pessoa enquanto você editava. Atualize a página e tente novamente."
	msgGroupFull            = "O grupo já atingiu o limite de integrantes."
	msgGroupFullWithPending = "O grupo já atingiu o limite de integrantes, considerando os convites pendentes."
	msgNotPreauthorized     = "Este RGM não está pré-autorizado e não tem conta de aluno."
	msgAlreadyInProject     = "Você já participa de um projeto."
	msgNoCohort             = "O seu RGM não está pré-autorizado em nenhuma turma. Procure a coordenação do curso."
	msgCohortWithoutProgram = "A sua turma ainda não tem programa de PFC. Procure a coordenação do curso."
	msgOutsideGroupCohort   = "Você não está pré-autorizado na turma deste grupo. Procure a coordenação do curso."
	msgInvitationNotFound   = "Convite não encontrado."
	msgGroupGone            = "Este grupo foi excluído."
	msgCannotJoin           = "Não foi possível entrar no grupo: ele já atingiu o limite de integrantes ou você já faz parte dele."
)

func msgAlreadyMember(rgm string) string  { return "O RGM " + rgm + " já é integrante do grupo." }
func msgNotMember(rgm string) string      { return "O RGM " + rgm + " não é integrante do grupo." }
func msgInOtherProject(rgm string) string { return "O RGM " + rgm + " já faz parte de outro projeto." }
func msgNotInCohort(rgm string) string {
	return "O RGM " + rgm + " não está pré-autorizado na sua turma. Peça à coordenação a pré-autorização."
}
func msgNotInGroupCohort(rgm string) string {
	return "O RGM " + rgm + " não está pré-autorizado na turma do grupo. Peça à coordenação a pré-autorização."
}
func msgAlreadyInvited(rgm string) string { return "O RGM " + rgm + " já tem um convite deste grupo." }
func msgInNamedProject(rgm, name string) string {
	return "O RGM " + rgm + " já faz parte do projeto \"" + name + "\"."
}
