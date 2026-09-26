package main

import (
	"context"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auth"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/matricula"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/programa"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/projeto"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/referencia"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/router"
)

var r *router.Router

func init() {
	r = router.New()

	// Autenticação
	r.Handle("POST", "/auth/login", auth.HandleLogin)
	r.Handle("POST", "/auth/registrar", auth.HandleRegistrar)

	// Matrículas
	r.Handle("GET", "/admin/students", matricula.HandleListar)
	r.Handle("POST", "/admin/students", matricula.HandleProvisionar)
	r.Handle("DELETE", "/admin/students", matricula.HandleRemover)

	// Programas
	r.Handle("POST", "/programas", programa.HandleCriar)
	r.Handle("GET", "/programas", programa.HandleListar)
	r.Handle("PUT", "/programas/:programaId", programa.HandleAtualizar)
	r.Handle("DELETE", "/programas/:programaId", programa.HandleDeletar)

	// Projetos
	r.Handle("POST", "/programas/:programaId/projetos", projeto.HandleCriar)
	r.Handle("GET", "/programas/:programaId/projetos", projeto.HandleListarPorPrograma)
	r.Handle("PUT", "/projetos/:projetoId", projeto.HandleAtualizar)
	r.Handle("DELETE", "/projetos/:projetoId", projeto.HandleDeletar)
	r.Handle("PUT", "/projetos/:projetoId/orientador", projeto.HandleAssociarOrientador)
	r.Handle("DELETE", "/projetos/:projetoId/orientador", projeto.HandleRemoverOrientador)
	r.Handle("PUT", "/projetos/:projetoId/integrantes", projeto.HandleAdicionarIntegrante)
	r.Handle("DELETE", "/projetos/:projetoId/integrantes", projeto.HandleRemoverIntegrante)

	// Referências bibliográficas (OpenAlex + Crossref)
	r.Handle("GET", "/referencias/busca", referencia.HandleBuscar)
	r.Handle("GET", "/referencias/doi", referencia.HandleConsultarDOI)
	r.Handle("GET", "/projetos/:projetoId/referencias", referencia.HandleListar)
	r.Handle("POST", "/projetos/:projetoId/referencias", referencia.HandleAdicionar)
	r.Handle("DELETE", "/projetos/:projetoId/referencias/:referenciaId", referencia.HandleRemover)
}

// Handler para API Gateway
func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return r.Dispatch(ctx, req)
}

func main() {
	lambda.Start(handler)
}