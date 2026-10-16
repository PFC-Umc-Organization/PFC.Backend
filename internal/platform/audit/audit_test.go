package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func capture(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	original := logger
	logger = slog.New(slog.NewJSONHandler(&buf, nil))
	t.Cleanup(func() { logger = original })
	return &buf
}

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, buf.String())
	}
	return line
}

var origin = Origin{
	Actor:     Actor{Sub: "sub-abc", Role: "ADMIN", Email: "coord@umc.br", IP: "203.0.113.5"},
	RequestID: "req-123",
}

func TestRecordUsesOrigin(t *testing.T) {
	buf := capture(t)
	Record(WithOrigin(context.Background(), origin), Event{
		Action:   "programa.criado",
		Result:   Success,
		Resource: &Resource{Type: "programa", ID: "prog-1"},
		Details:  map[string]any{"cursoId": "c-eng-noite"},
	})

	line := decode(t, buf)
	if line["tipo"] != "auditoria" || line["acao"] != "programa.criado" || line["resultado"] != "sucesso" || line["requestId"] != "req-123" {
		t.Errorf("line = %v", line)
	}
	actor, _ := line["ator"].(map[string]any)
	if actor["sub"] != "sub-abc" || actor["perfil"] != "ADMIN" || actor["ip"] != "203.0.113.5" {
		t.Errorf("ator = %v", actor)
	}
	resource, _ := line["recurso"].(map[string]any)
	if resource["tipo"] != "programa" || resource["id"] != "prog-1" {
		t.Errorf("recurso = %v", line["recurso"])
	}
}

func TestRecordOmitsEmptyFields(t *testing.T) {
	buf := capture(t)
	Record(WithOrigin(context.Background(), origin), Event{Action: "programa.criado", Result: Failure, Reason: "acesso restrito a administradores"})

	line := decode(t, buf)
	if line["motivo"] != "acesso restrito a administradores" {
		t.Errorf("motivo = %v", line["motivo"])
	}
	for _, k := range []string{"recurso", "detalhes"} {
		if _, ok := line[k]; ok {
			t.Errorf("%s should be omitted: %v", k, line[k])
		}
	}
}

func TestRecordExplicitActorKeepsOriginIP(t *testing.T) {
	buf := capture(t)
	Record(WithOrigin(context.Background(), origin), Event{Action: "auth.login", Result: Success, Actor: &Actor{Email: "a@alunos.umc.br"}})

	actor, _ := decode(t, buf)["ator"].(map[string]any)
	if actor["email"] != "a@alunos.umc.br" || actor["ip"] != "203.0.113.5" || actor["sub"] != nil {
		t.Errorf("ator = %v", actor)
	}
}
