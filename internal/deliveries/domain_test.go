package deliveries

import (
	"strings"
	"testing"
)

func TestActivityInputValidation(t *testing.T) {
	if msg := (ActivityInput{Title: "  Tema  ", Description: "d", Deadline: "2026-10-15T23:59"}).normalized().validate(); msg != "" {
		t.Errorf("valid input refused: %s", msg)
	}
	for name, in := range map[string]ActivityInput{
		"no title":       {Title: "  ", Deadline: "2026-10-15T23:59"},
		"bad deadline":   {Title: "Tema", Deadline: "15/10/2026"},
		"title too long": {Title: strings.Repeat("a", maxTitleLen+1), Deadline: "2026-10-15T23:59"},
	} {
		if in.normalized().validate() == "" {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestFieldInputValidation(t *testing.T) {
	if f, msg := (FieldInput{Label: " Repo ", Type: FieldLink}).normalized(); msg != "" || f.Label != "Repo" {
		t.Errorf("valid field refused: %q %s", f.Label, msg)
	}
	if _, msg := (FieldInput{Label: "", Type: FieldLink}).normalized(); msg == "" {
		t.Error("blank label must fail")
	}
	if _, msg := (FieldInput{Label: "x", Type: "XYZ"}).normalized(); msg == "" {
		t.Error("unknown type must fail")
	}
}

func TestNormalizedFields(t *testing.T) {
	fields, msg := normalizedFields(nil)
	if msg != "" || len(fields) != 1 || fields[0].Type != FieldFile || !fields[0].Required {
		t.Errorf("default = %+v (%s)", fields, msg)
	}
	if _, msg := normalizedFields([]FieldInput{{Label: "Repo", Type: FieldLink}, {Label: "Resumo", Type: FieldLongText}}); msg != "" {
		t.Errorf("valid fields refused: %s", msg)
	}
	if _, msg := normalizedFields([]FieldInput{{Label: "", Type: FieldLink}}); msg == "" {
		t.Error("invalid field must fail")
	}
	if _, msg := normalizedFields(make([]FieldInput, maxFields+1)); msg == "" {
		t.Error("too many fields must fail")
	}
}

func TestValidAnswers(t *testing.T) {
	fields := []Field{
		{ID: "c1", Label: "Arquivo", Type: FieldFile, Required: true},
		{ID: "c2", Label: "Repo", Type: FieldLink},
		{ID: "c3", Label: "Resumo", Type: FieldText},
	}
	answers, msg := validAnswers(fields, map[string]string{"c1": " tema.pdf ", "c2": "https://github.com/x/y", "c3": "  "})
	if msg != "" || answers["c1"] != "tema.pdf" || len(answers) != 2 {
		t.Fatalf("answers = %v (%s)", answers, msg)
	}
	for name, a := range map[string]map[string]string{
		"required missing":  {"c2": "https://x.com"},
		"unknown field":     {"c1": "a.pdf", "zzz": "x"},
		"link without http": {"c1": "a.pdf", "c2": "github.com/x"},
		"text too long":     {"c1": "a.pdf", "c3": strings.Repeat("a", maxTextLen+1)},
	} {
		if _, msg := validAnswers(fields, a); msg == "" {
			t.Errorf("%s: expected an error", name)
		}
	}
}
