package deliveries

import (
	"path"
	"strings"
)

const (
	filesPrefix   = "entregas/"
	msgNoStorage  = "O envio de arquivos ainda não está disponível."
	msgNotAFile   = "Este campo não aceita arquivo."
	msgFileType   = "Tipo de arquivo não aceito. Envie PDF, documento do Office/LibreOffice, imagem, ZIP ou texto."
	msgFileSize   = "O arquivo excede o limite de 50 MB."
	msgFileEmpty  = "O arquivo está vazio."
	msgNoFileHere = "Esta entrega não tem arquivo para download."
)

// Entrada
type UploadRequest struct {
	FieldID string `json:"campoId"`
	Name    string `json:"nome"`
	Size    int64  `json:"tamanho"`
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

// Chave
func fieldPrefix(projectID, activityID, fieldID string) string {
	return filesPrefix + projectID + "/" + activityID + "/" + fieldID + "/"
}

// Verificação
func isStoredFile(answer string) bool { return strings.HasPrefix(answer, filesPrefix) }

// Extração
func fileNameOf(key string) string { return path.Base(key) }
