package router

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestDispatch(t *testing.T) {
	r := New()
	var recebido string
	r.Handle("GET", "/projetos/:projetoId/entregas", func(_ context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		recebido = req.PathParameters["projetoId"]
		return events.APIGatewayProxyResponse{StatusCode: 200}, nil
	})

	resp, _ := r.Dispatch(context.Background(), events.APIGatewayProxyRequest{HTTPMethod: "GET", Path: "/projetos/abc/entregas"})
	if resp.StatusCode != 200 || recebido != "abc" {
		t.Fatalf("status %d, projetoId %q", resp.StatusCode, recebido)
	}

	// O 404 precisa dos headers de CORS, senão o navegador não deixa o
	// front ler a resposta.
	resp, _ = r.Dispatch(context.Background(), events.APIGatewayProxyRequest{HTTPMethod: "GET", Path: "/nao-existe"})
	if resp.StatusCode != 404 || resp.Headers["Access-Control-Allow-Origin"] != "*" {
		t.Fatalf("404 sem CORS: status %d, headers %v", resp.StatusCode, resp.Headers)
	}
}
