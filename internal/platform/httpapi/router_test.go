package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
)

func call(t *testing.T, rt *Router, ev events.APIGatewayProxyRequest) (int, string, map[string]string) {
	t.Helper()
	resp, err := rt.Lambda(context.Background(), ev)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]string
	_ = json.Unmarshal([]byte(resp.Body), &body)
	return resp.StatusCode, body["erro"], resp.Headers
}

func TestRouting(t *testing.T) {
	rt := NewRouter()
	var got *Request
	rt.Handle("GET", "/projetos/:projetoId/entregas", func(_ context.Context, r *Request) (Response, error) {
		got = r
		return JSON(200, map[string]string{}), nil
	})

	status, _, headers := call(t, rt, events.APIGatewayProxyRequest{
		HTTPMethod:            "GET",
		Path:                  "/projetos/abc/entregas",
		QueryStringParameters: map[string]string{"q": "x"},
		RequestContext: events.APIGatewayProxyRequestContext{Authorizer: map[string]any{
			"claims": map[string]any{"sub": "s1", "email": "123@alunos.umc.br", "custom:perfil": "ALUNO"},
		}},
	})
	if status != 200 || got.Param("projetoId") != "abc" || got.Query("q") != "x" {
		t.Fatalf("status %d, request %+v", status, got)
	}
	if got.Actor != (actor.Actor{Sub: "s1", Email: "123@alunos.umc.br", Role: actor.RoleStudent}) {
		t.Fatalf("actor %+v", got.Actor)
	}
	if headers["Content-Type"] != "application/json" || headers["Access-Control-Allow-Origin"] != "*" {
		t.Fatalf("headers %v", headers)
	}

	// Without CORS headers the browser hides the 404 behind a CORS error.
	status, msg, headers := call(t, rt, events.APIGatewayProxyRequest{HTTPMethod: "GET", Path: "/nao-existe"})
	if status != 404 || msg != "rota não encontrada" || headers["Access-Control-Allow-Origin"] != "*" {
		t.Fatalf("404: status %d, msg %q, headers %v", status, msg, headers)
	}

	status, _, _ = call(t, rt, events.APIGatewayProxyRequest{HTTPMethod: "POST", Path: "/projetos/abc/entregas"})
	if status != 404 {
		t.Fatalf("wrong method: status %d", status)
	}
}

func TestPreflight(t *testing.T) {
	resp, _ := NewRouter().Lambda(context.Background(), events.APIGatewayProxyRequest{HTTPMethod: "OPTIONS", Path: "/x"})
	if resp.StatusCode != 204 || resp.Headers["Access-Control-Allow-Methods"] == "" || resp.Body != "" {
		t.Fatalf("preflight %+v", resp)
	}
}

func TestJWTPayloadClaims(t *testing.T) {
	a := ActorOf(events.APIGatewayProxyRequest{RequestContext: events.APIGatewayProxyRequestContext{
		Authorizer: map[string]any{"jwt": map[string]any{"claims": map[string]any{"sub": "s2", "custom:perfil": "ADMIN"}}},
	}})
	if a.Sub != "s2" || a.Role != actor.RoleAdmin {
		t.Fatalf("actor %+v", a)
	}
	if (ActorOf(events.APIGatewayProxyRequest{}) != actor.Actor{}) {
		t.Fatal("no authorizer must yield an empty actor")
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		err    error
		status int
		msg    string
	}{
		{apperr.Invalid("nome é obrigatório"), 400, "nome é obrigatório"},
		{apperr.Forbidden("acesso restrito a administradores"), 403, "acesso restrito a administradores"},
		{apperr.NotFound("turma não encontrada"), 404, "turma não encontrada"},
		{apperr.Conflict("já existe"), 409, "já existe"},
		{apperr.Unavailable("indisponível", errors.New("timeout")), 502, "indisponível"},
		{apperr.Internal("falha ao listar turmas", errors.New("boom")), 500, "falha ao listar turmas"},
		{errors.New("unexpected"), 500, msgInternal},
	}
	for _, c := range cases {
		rt := NewRouter()
		rt.Handle("GET", "/x", func(context.Context, *Request) (Response, error) { return Response{}, c.err })
		if status, msg, _ := call(t, rt, events.APIGatewayProxyRequest{HTTPMethod: "GET", Path: "/x"}); status != c.status || msg != c.msg {
			t.Errorf("%v: got %d %q, want %d %q", c.err, status, msg, c.status, c.msg)
		}
	}
}

func TestPanicBecomes500(t *testing.T) {
	rt := NewRouter()
	rt.Handle("GET", "/x", func(context.Context, *Request) (Response, error) { panic("boom") })
	if status, msg, _ := call(t, rt, events.APIGatewayProxyRequest{HTTPMethod: "GET", Path: "/x"}); status != 500 || msg != msgInternal {
		t.Fatalf("got %d %q", status, msg)
	}
}

func TestDecode(t *testing.T) {
	var v struct{ Nome string }
	r := &Request{Body: `{"nome":"x"}`}
	if err := r.Decode(&v); err != nil || v.Nome != "x" {
		t.Fatalf("decode: %v %+v", err, v)
	}
	err := (&Request{Body: "{"}).Decode(&v)
	if e, ok := apperr.As(err); !ok || e.Kind != apperr.KindInvalid || e.Message != msgInvalidBody {
		t.Fatalf("invalid body: %v", err)
	}
}

func TestLegacy(t *testing.T) {
	rt := NewRouter()
	rt.Handle("DELETE", "/turmas/:turmaId", Legacy(func(_ context.Context, ev events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: `{"id":"` + ev.PathParameters["turmaId"] + `"}`}, nil
	}))
	resp, _ := rt.Lambda(context.Background(), events.APIGatewayProxyRequest{HTTPMethod: "DELETE", Path: "/turmas/t1"})
	if resp.StatusCode != 200 || resp.Body != `{"id":"t1"}` || resp.Headers["Content-Type"] != "application/json" {
		t.Fatalf("legacy %+v", resp)
	}
}
