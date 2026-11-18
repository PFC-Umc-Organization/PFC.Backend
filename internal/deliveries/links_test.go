package deliveries

import (
	"context"
	"strings"
	"testing"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/storage"
)

// ---------------------------------------------------------------------------
// Formatos
// ---------------------------------------------------------------------------

// Normalização
func TestNormalizedFormat(t *testing.T) {
	cases := []struct {
		kind   FieldType
		in     string
		want   string
		failed bool
	}{
		{FieldFile, "", "GERAL", false},
		{FieldFile, " slides ", "SLIDES", false},
		{FieldFile, "PLANILHA", "PLANILHA", false},
		{FieldFile, "REPOSITORIO", "", true},
		{FieldLink, "", "GERAL", false},
		{FieldLink, "video", "VIDEO", false},
		{FieldLink, "SLIDES", "", true},
		{FieldText, "DOCUMENTO", "", false},
	}
	for _, c := range cases {
		got, msg := normalizedFormat(c.kind, c.in)
		if got != c.want || (msg != "") != c.failed {
			t.Errorf("%s %q: got %q, %q", c.kind, c.in, got, msg)
		}
	}
}

// Extensões
func TestFileFormats(t *testing.T) {
	cases := []struct {
		format, name string
		want         bool
	}{
		{"GERAL", "a.zip", true},
		{"", "a.png", true},
		{"DOCUMENTO", "tcc.PDF", true},
		{"DOCUMENTO", "tcc.pptx", false},
		{"SLIDES", "aula.pdf", true},
		{"SLIDES", "aula.docx", false},
		{"PLANILHA", "dados.csv", true},
		{"PLANILHA", "dados.pdf", false},
		{"COMPACTADO", "codigo.tar.gz", true},
		{"COMPACTADO", "codigo.gz", false},
		{"COMPACTADO", "codigo.7z", true},
		{"COMPACTADO", "codigo.pdf", false},
	}
	for _, c := range cases {
		if got := storage.FileFormatAllows(c.format, c.name); got != c.want {
			t.Errorf("%s %s: got %v", c.format, c.name, got)
		}
	}
}

// Entrega
func TestSubmitFormats(t *testing.T) {
	activity := Activity{ID: "a1", Fields: []Field{
		{ID: "f1", Label: "Texto", Type: FieldFile, Format: "DOCUMENTO"},
		{ID: "f2", Label: "Código", Type: FieldLink, Format: FormatRepository},
	}}
	member := actor.Actor{Role: actor.RoleStudent, Email: "111@alunos.umc.br", Name: "Ana"}

	cases := map[string]struct {
		answers map[string]string
		want    string
	}{
		"valid":        {map[string]string{"f1": "tcc.docx", "f2": "https://github.com/umc/pfc"}, ""},
		"wrong file":   {map[string]string{"f1": "tcc.zip"}, `O campo "Texto" aceita apenas arquivos .pdf, .doc, .docx, .odt.`},
		"wrong host":   {map[string]string{"f2": "https://exemplo.com.br/umc/pfc"}, `O campo "Código" aceita apenas links do GitHub, GitLab ou Bitbucket.`},
		"missing repo": {map[string]string{"f2": "https://github.com/umc"}, `O campo "Código" precisa do link do repositório, no formato usuario/repositorio.`},
		"shortener":    {map[string]string{"f2": "https://bit.ly/abc123"}, msgShortener},
		"localhost":    {map[string]string{"f2": "http://localhost:8080/a/b"}, `O campo "Código" precisa de um link válido, com endereço público iniciado por http:// ou https://.`},
		"adult":        {map[string]string{"f2": "https://pornhub.com/a/b"}, msgBlockedDomain},
	}
	for name, c := range cases {
		store := &fakeStore{activities: map[string]Activity{"a1": activity}}
		_, err := newTestService(t, store).Submit(context.Background(), member, "p1", "a1", SubmissionInput{Answers: c.answers})
		if message(err) != c.want {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

// Ticket
func TestUploadFormat(t *testing.T) {
	activity := Activity{ID: "a1", Fields: []Field{{ID: "f1", Label: "Planilha", Type: FieldFile, Format: "PLANILHA"}}}
	member := actor.Actor{Role: actor.RoleStudent, Email: "111@alunos.umc.br"}
	svc := newTestService(t, &fakeStore{activities: map[string]Activity{"a1": activity}})
	svc.files = &fakeFiles{sizes: map[string]int64{}}

	if _, err := svc.RequestUpload(context.Background(), member, "p1", "a1", UploadRequest{FieldID: "f1", Name: "dados.xlsx", Size: 10}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.RequestUpload(context.Background(), member, "p1", "a1", UploadRequest{FieldID: "f1", Name: "dados.pdf", Size: 10})
	if kind(err) != apperr.KindInvalid || !strings.Contains(message(err), ".xls, .xlsx, .ods, .csv") {
		t.Errorf("wrong extension: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Links
// ---------------------------------------------------------------------------

// Validação
func TestCheckLink(t *testing.T) {
	general := Field{Label: "Link", Type: FieldLink}
	video := Field{Label: "Vídeo", Type: FieldLink, Format: FormatVideo}
	cases := []struct {
		field Field
		url   string
		ok    bool
	}{
		{general, "https://exemplo.com.br/pasta", true},
		{general, "http://www.exemplo.org", true},
		{general, "ftp://exemplo.com", false},
		{general, "https://localhost/x", false},
		{general, "http://127.0.0.1/x", false},
		{general, "http://192.168.0.10/x", false},
		{general, "https://intranet/x", false},
		{general, "https://exemplo.c/x", false},
		{general, "https://user:senha@exemplo.com/x", false},
		{general, "https://bit.ly/x", false},
		{general, "https://www.tinyurl.com/x", false},
		{general, "https://t.co/x", false},
		{general, "https://rb.gy/x", false},
		{general, "https://xvideos.com/x", false},
		{general, "https://meu-nsfw-site.com/x", false},
		{video, "https://www.youtube.com/watch?v=abc", true},
		{video, "https://youtu.be/abc", true},
		{video, "https://youtube.com/", false},
		{video, "https://vimeo.com/12345", true},
		{video, "https://github.com/a/b", false},
	}
	for _, c := range cases {
		if got := checkLink(c.field, c.url) == ""; got != c.ok {
			t.Errorf("%s %s: got %v", c.field.Format, c.url, got)
		}
	}
}
