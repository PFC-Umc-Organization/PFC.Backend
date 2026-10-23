// Package materials owns the support materials the academic team publishes:
// links or uploaded files, for one cohort or for everyone.
package materials

import (
	"strings"
	"unicode/utf8"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
)

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

// A material has a URL or a file; File is the file name shown to people,
// the key stays in the store. CohortID nil means every cohort.
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

// MaterialInput.File is the key returned by POST /materiais/arquivos.
type MaterialInput struct {
	Title       string  `json:"titulo"`
	Description string  `json:"descricao"`
	Kind        Kind    `json:"tipo"`
	URL         string  `json:"url"`
	File        string  `json:"arquivo"`
	CohortID    *string `json:"cursoId"`
}

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

type UploadRequest struct {
	Name string `json:"nome"`
	Size int64  `json:"tamanho"`
}

// UploadTicket is sent as a multipart form to URL: Fields first, the file
// last, under "file". Key goes in MaterialInput.File.
type UploadTicket struct {
	URL       string            `json:"url"`
	Fields    map[string]string `json:"campos"`
	Key       string            `json:"chave"`
	ExpiresAt string            `json:"expiraEm"`
	MaxBytes  int64             `json:"tamanhoMaximo"`
}

type Download struct {
	URL  string `json:"url"`
	Name string `json:"nome"`
}

func isStaff(a actor.Actor) bool { return a.HasRole(actor.RoleAdmin, actor.RoleAdvisor) }

// visibleTo: the team sees everything; a student sees the general materials
// and their cohort's.
func (m Material) visibleTo(a actor.Actor, cohort string) bool {
	return isStaff(a) || m.CohortID == nil || (cohort != "" && *m.CohortID == cohort)
}

// canRemove: the coordination removes any material, an advisor their own.
func (m Material) canRemove(a actor.Actor) bool {
	return a.HasRole(actor.RoleAdmin) || (a.HasRole(actor.RoleAdvisor) && a.Sub != "" && m.AuthorID == a.Sub)
}

const (
	msgStaffOnly          = "Acesso restrito à equipe acadêmica"
	msgRemoveOwn          = "O orientador só remove os materiais que publicou"
	msgTitleRequired      = "Título é obrigatório"
	msgTitleTooLong       = "Título longo demais"
	msgDescriptionTooLong = "Descrição longa demais"
	msgInvalidKind        = "Tipo de material inválido"
	msgURLOrFile          = "Informe um link ou envie um arquivo"
	msgInvalidURL         = "Link inválido. Use um endereço http(s)"
	msgUploadFirst        = "Envie o arquivo pelo botão de envio"
	msgFileMissing        = "O arquivo não chegou. Envie de novo"
	msgCohortNotFound     = "Turma não encontrada"
	msgNotFound           = "Material não encontrado"
	msgNoFile             = "Este material não tem arquivo"
	msgNoStorage          = "O envio de arquivos ainda não está disponível"
	msgFileType           = "Tipo de arquivo não aceito. Envie PDF, documento do Office/LibreOffice, imagem, ZIP ou texto"
	msgFileSize           = "O arquivo passa de 50 MB"
	msgFileEmpty          = "O arquivo está vazio"
)
