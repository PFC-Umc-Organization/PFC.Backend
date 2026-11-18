package storage

import (
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxBytes = 50 << 20

const maxNameLen = 120

var contentTypes = map[string]string{
	".pdf":    "application/pdf",
	".doc":    "application/msword",
	".docx":   "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".odt":    "application/vnd.oasis.opendocument.text",
	".ppt":    "application/vnd.ms-powerpoint",
	".pptx":   "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".odp":    "application/vnd.oasis.opendocument.presentation",
	".xls":    "application/vnd.ms-excel",
	".xlsx":   "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".ods":    "application/vnd.oasis.opendocument.spreadsheet",
	".csv":    "text/csv",
	".zip":    "application/zip",
	".rar":    "application/vnd.rar",
	".7z":     "application/x-7z-compressed",
	".tar.gz": "application/gzip",
	".png":    "image/png",
	".jpg":    "image/jpeg",
	".jpeg":   "image/jpeg",
	".txt":    "text/plain",
}

// Mapeamento
func ContentType(name string) (string, bool) {
	t, ok := contentTypes[Ext(name)]
	return t, ok
}

// Extensão
func Ext(name string) string {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".tar.gz") {
		return ".tar.gz"
	}
	return path.Ext(lower)
}

// ---------------------------------------------------------------------------
// Formatos
// ---------------------------------------------------------------------------

const FormatGeneral = "GERAL"

var fileFormats = map[string][]string{
	"DOCUMENTO":  {".pdf", ".doc", ".docx", ".odt"},
	"SLIDES":     {".ppt", ".pptx", ".odp", ".pdf"},
	"PLANILHA":   {".xls", ".xlsx", ".ods", ".csv"},
	"COMPACTADO": {".zip", ".rar", ".7z", ".tar.gz"},
}

// Consulta
func FileFormatKnown(format string) bool {
	_, ok := fileFormats[format]
	return format == FormatGeneral || ok
}

// Consulta
func FileFormatExts(format string) []string { return fileFormats[format] }

// Verificação
func FileFormatAllows(format, name string) bool {
	exts, restricted := fileFormats[format]
	if !restricted {
		return true
	}
	return slices.Contains(exts, Ext(name))
}

// Sanitização
func SafeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	if utf8.RuneCountInString(name) <= maxNameLen {
		return name
	}
	ext := path.Ext(name)
	if utf8.RuneCountInString(ext) > 10 {
		ext = ""
	}
	runes := []rune(strings.TrimSuffix(name, ext))
	return string(runes[:maxNameLen-utf8.RuneCountInString(ext)]) + ext
}
