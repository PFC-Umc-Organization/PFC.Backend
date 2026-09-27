package auditoria

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func comAtorFalso() events.APIGatewayProxyRequest {
	req := events.APIGatewayProxyRequest{
		RequestContext: events.APIGatewayProxyRequestContext{
			RequestID: "req-123",
			Authorizer: map[string]interface{}{
				"claims": map[string]interface{}{
					"sub":           "sub-abc",
					"custom:perfil": "COORDENADOR",
					"email":         "coord@umc.br",
				},
			},
		},
	}
	req.RequestContext.Identity.SourceIP = "203.0.113.5"
	return req
}

func TestRegistrarEventoDeSucesso(t *testing.T) {
	var buf bytes.Buffer
	original := logger
	logger = slog.New(slog.NewJSONHandler(&buf, nil))
	defer func() { logger = original }()

	Registrar(context.Background(), Evento{
		Acao:      "programa.criado",
		Resultado: "sucesso",
		Recurso:   &Recurso{Tipo: "programa", ID: "prog-1"},
		Detalhes:  map[string]any{"cursoId": "c-eng-noite"},
	}, comAtorFalso())

	var linha map[string]any
	if err := json.Unmarshal(buf.Bytes(), &linha); err != nil {
		t.Fatalf("linha de log não é JSON válido: %v\nsaída: %s", err, buf.String())
	}

	if linha["tipo"] != "auditoria" {
		t.Errorf("tipo = %v, esperado \"auditoria\"", linha["tipo"])
	}
	if linha["acao"] != "programa.criado" {
		t.Errorf("acao = %v, esperado \"programa.criado\"", linha["acao"])
	}
	if linha["resultado"] != "sucesso" {
		t.Errorf("resultado = %v, esperado \"sucesso\"", linha["resultado"])
	}
	if linha["requestId"] != "req-123" {
		t.Errorf("requestId = %v, esperado \"req-123\"", linha["requestId"])
	}

	ator, ok := linha["ator"].(map[string]any)
	if !ok {
		t.Fatalf("ator não veio como objeto: %v", linha["ator"])
	}
	if ator["sub"] != "sub-abc" || ator["perfil"] != "COORDENADOR" || ator["ip"] != "203.0.113.5" {
		t.Errorf("ator incompleto: %v", ator)
	}

	recurso, ok := linha["recurso"].(map[string]any)
	if !ok || recurso["tipo"] != "programa" || recurso["id"] != "prog-1" {
		t.Errorf("recurso incorreto: %v", linha["recurso"])
	}
}

func TestRegistrarEventoDeFalhaOmiteRecurso(t *testing.T) {
	var buf bytes.Buffer
	original := logger
	logger = slog.New(slog.NewJSONHandler(&buf, nil))
	defer func() { logger = original }()

	Registrar(context.Background(), Evento{
		Acao:      "programa.criado",
		Resultado: "falha",
		Motivo:    "acesso restrito a coordenadores",
	}, comAtorFalso())

	var linha map[string]any
	if err := json.Unmarshal(buf.Bytes(), &linha); err != nil {
		t.Fatalf("linha de log não é JSON válido: %v", err)
	}

	if linha["motivo"] != "acesso restrito a coordenadores" {
		t.Errorf("motivo = %v", linha["motivo"])
	}
	if _, existe := linha["recurso"]; existe {
		t.Errorf("recurso não deveria aparecer quando nil, veio: %v", linha["recurso"])
	}
}
