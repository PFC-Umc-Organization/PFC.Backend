package materials

import (
	"strings"
	"unicode/utf8"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
)

// ---------------------------------------------------------------------------
// Material
// ---------------------------------------------------------------------------

// Enumeração
type Kind string

const (
	KindDocument Kind = "DOCUMENTO"
	KindTemplate Kind = "MODELO"
	KindVideo    Kind = "VIDEO"
	KindLink     Kind = "LINK"
)

const (
	maxTitleLen       = 150
	maxDescriptionLen = 2000
	maxURLLen         = 2000
	filesPrefix       = "materiais/"
)

// Modelo
type Material struct {
	ID          string  `json:"id"`
	Title       string  `json:"titulo"`
	Description string  `json:"descricao"`
	Kind        Kind    `json:"tipo"`
	URL         string  `json:"url,omitempty"`
	File        string  `json:"arquivo,omitempty"`
	CohortID    *string `json:"cursoId"`
	PublishedAt string  `json:"publicadoEm"`
	PublishedBy string  `json:"publicadoPor"`
	AuthorID    string  `json:"autorId"`

	fileKey string
}

// Entrada
type MaterialInput struct {
	Title       string  `json:"titulo"`
	Description string  `json:"descricao"`
	Kind        Kind    `json:"tipo"`
	URL         string  `json:"url"`
	File        string  `json:"arquivo"`
	CohortID    *string `json:"cursoId"`
}

// Normalização
func (in MaterialInput) normalized() MaterialInput {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.URL = strings.TrimSpace(in.URL)
	in.File = strings.TrimSpace(in.File)
	if in.CohortID != nil {
		if id := strings.TrimSpace(*in.CohortID); id == "" {
			in.CohortID = nil
		} else {
			in.CohortID = &id
		}
	}
	return in
}

// Validação
func (in MaterialInput) validate() string {
	switch {
	case in.Title == "":
		return msgTitleRequired
	case utf8.RuneCountInString(in.Title) > maxTitleLen:
		return msgTitleTooLong
	case utf8.RuneCountInString(in.Description) > maxDescriptionLen:
		return msgDescriptionTooLong
	}
	switch in.Kind {
	case KindDocument, KindTemplate, KindVideo, KindLink:
	default:
		return msgInvalidKind
	}
	switch {
	case (in.URL == "") == (in.File == ""):
		return msgURLOrFile
	case in.URL != "" && !strings.HasPrefix(in.URL, "https://") && !strings.HasPrefix(in.URL, "http://"):
		return msgInvalidURL
	case utf8.RuneCountInString(in.URL) > maxURLLen:
		return msgInvalidURL
	case in.File != "" && !strings.HasPrefix(in.File, filesPrefix):
		return msgUploadFirst
	}
	return ""
}

// ---------------------------------------------------------------------------
// Arquivos
// ---------------------------------------------------------------------------

// Entrada
type UploadRequest struct {
	Name string `json:"nome"`
	Size int64  `json:"tamanho"`
}

// Resposta
type UploadTicket struct {
	URL       string            `json:"url"`
	Fields    map[string]string `json:"campos"`
	Key       string            `json:"chave"`
	ExpiresAt string            `json:"expiraEm"`
	MaxBytes  int64             `json:"tamanhoMaximo"`
}

// Resposta
type Download struct {
	URL  string `json:"url"`
	Name string `json:"nome"`
}

// ---------------------------------------------------------------------------
// Permissões
// ---------------------------------------------------------------------------

// Verificação
func isStaff(a actor.Actor) bool { return a.HasRole(actor.RoleAdmin, actor.RoleAdvisor) }

// Verificação
func (m Material) visibleTo(a actor.Actor, cohort string) bool {
	return isStaff(a) || m.CohortID == nil || (cohort != "" && *m.CohortID == cohort)
}

// Permissão
func (m Material) canRemove(a actor.Actor) bool {
	return a.HasRole(actor.RoleAdmin) || (a.HasRole(actor.RoleAdvisor) && a.Sub != "" && m.AuthorID == a.Sub)
}

const (
	msgStaffOnly          = "Acesso restrito a orientadores e à coordenação."
	msgRemoveOwn          = "Orientadores só podem remover os materiais que publicaram."
	msgTitleRequired      = "Informe o título."
	msgTitleTooLong       = "O título excede o tamanho máximo permitido."
	msgDescriptionTooLong = "A descrição excede o tamanho máximo permitido."
	msgInvalidKind        = "Tipo de material inválido."
	msgURLOrFile          = "Informe um link ou envie um arquivo."
	msgInvalidURL         = "Link inválido. Informe um endereço iniciado por http:// ou https://."
	msgUploadFirst        = "Use o botão Escolher arquivo para enviar o arquivo."
	msgFileMissing        = "O arquivo não foi recebido. Envie-o novamente."
	msgCohortNotFound     = "Turma não encontrada."
	msgNotFound           = "Material não encontrado."
	msgNoFile             = "Este material não tem arquivo para download."
	msgNoStorage          = "O envio de arquivos ainda não está disponível."
	msgFileType           = "Tipo de arquivo não aceito. Envie PDF, documento do Office/LibreOffice, imagem, ZIP ou texto."
	msgFileSize           = "O arquivo excede o limite de 50 MB."
	msgFileEmpty          = "O arquivo está vazio."
)
