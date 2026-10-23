package deliveries

import (
	"path"
	"strings"
)

const (
	filesPrefix   = "entregas/"
	msgNoStorage  = "o envio de arquivos ainda não está disponível"
	msgNotAFile   = "este campo não recebe arquivo"
	msgFileType   = "tipo de arquivo não aceito — envie PDF, documento do Office/LibreOffice, imagem, ZIP ou texto"
	msgFileSize   = "o arquivo passa de 50 MB"
	msgFileEmpty  = "o arquivo está vazio"
	msgNoFileHere = "esta entrega não tem arquivo para baixar"
)

type UploadRequest struct {
	FieldID string `json:"campoId"`
	Name    string `json:"nome"`
	Size    int64  `json:"tamanho"`
}

// UploadTicket is sent as a multipart form to URL: Fields first, the file
// last, under "file". Key is the answer to submit for the field.
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

func fieldPrefix(projectID, activityID, fieldID string) string {
	return filesPrefix + projectID + "/" + activityID + "/" + fieldID + "/"
}

// isStoredFile tells an uploaded file from the bare names saved before
// storage existed.
func isStoredFile(answer string) bool { return strings.HasPrefix(answer, filesPrefix) }

func fileNameOf(key string) string { return path.Base(key) }
