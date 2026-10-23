package storage

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxBytes is the size limit of any uploaded file.
const MaxBytes = 50 << 20

const maxNameLen = 120

// fileTypes decides the Content-Type from the extension; the browser's own
// guess varies between systems.
var contentTypes = map[string]string{
	".pdf":  "application/pdf",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".odt":  "application/vnd.oasis.opendocument.text",
	".ppt":  "application/vnd.ms-powerpoint",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".odp":  "application/vnd.oasis.opendocument.presentation",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".ods":  "application/vnd.oasis.opendocument.spreadsheet",
	".zip":  "application/zip",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".txt":  "text/plain",
}

func ContentType(name string) (string, bool) {
	t, ok := contentTypes[strings.ToLower(path.Ext(name))]
	return t, ok
}

// SafeName keeps the name readable in the key and in the download: no path
// separators or control characters, and a bounded length that preserves the
// extension.
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
