package deliveries

import (
	"fmt"
	"strings"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/storage"
)

// ---------------------------------------------------------------------------
// Formatos
// ---------------------------------------------------------------------------

const (
	FormatGeneral    = "GERAL"
	FormatRepository = "REPOSITORIO"
	FormatVideo      = "VIDEO"
)

// Normalização
func normalizedFormat(t FieldType, format string) (string, string) {
	format = strings.ToUpper(strings.TrimSpace(format))
	switch t {
	case FieldFile:
		if format == "" {
			return FormatGeneral, ""
		}
		if !storage.FileFormatKnown(format) {
			return "", msgFileFormatInvalid
		}
		return format, ""
	case FieldLink:
		switch format {
		case "":
			return FormatGeneral, ""
		case FormatGeneral, FormatRepository, FormatVideo:
			return format, ""
		}
		return "", msgLinkFormatInvalid
	}
	return "", ""
}

// Mensagem
func msgFormatFile(f Field) string {
	return fmt.Sprintf("O campo %q aceita apenas arquivos %s.", f.Label, strings.Join(storage.FileFormatExts(f.Format), ", "))
}

// Validação
func checkAnswers(fields []Field, answers, previous map[string]string) string {
	for _, f := range fields {
		v, ok := answers[f.ID]
		if !ok || v == previous[f.ID] {
			continue
		}
		switch f.Type {
		case FieldFile:
			if !storage.FileFormatAllows(f.Format, fileNameOf(v)) {
				return msgFormatFile(f)
			}
		case FieldLink:
			if msg := checkLink(f, v); msg != "" {
				return msg
			}
		}
	}
	return ""
}

const (
	msgFileFormatInvalid = "Formato de arquivo inválido para o campo."
	msgLinkFormatInvalid = "Formato de link inválido para o campo."
)
