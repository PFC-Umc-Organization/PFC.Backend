package main

import (
	"context"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auth"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/matricula"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/programa"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/projeto"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/router"
)

var r *router.Router

func init() {
	r = router.New()

	r.Handle("POST", "/auth/login", auth.HandleLogin)
	r.Handle("POST", "/auth/registrar", auth.HandleRegistrar)

	r.Handle("GET", "/admin/students", matricula.HandleListar)
	r.Handle("POST", "/admin/students", matricula.HandleProvisionar)
	r.Handle("DELETE", "/admin/students", matricula.HandleRemover)

	r.Handle("POST", "/programas", programa.HandleCriar)
	r.Handle("GET", "/programas", programa.HandleListar)
	r.Handle("PUT", "/programas/:programaId", programa.HandleAtualizar)
	r.Handle("DELETE", "/programas/:programaId", programa.HandleDeletar)

	r.Handle("POST", "/programas/:programaId/projetos", projeto.HandleCriar)
	r.Handle("GET", "/programas/:programaId/projetos", projeto.HandleListarPorPrograma)
	r.Handle("PUT", "/projetos/:projetoId", projeto.HandleAtualizar)
	r.Handle("DELETE", "/projetos/:projetoId", projeto.HandleDeletar)
	r.Handle("PUT", "/projetos/:projetoId/orientador", projeto.HandleAssociarOrientador)
	r.Handle("DELETE", "/projetos/:projetoId/orientador", projeto.HandleRemoverOrientador)
	r.Handle("PUT", "/projetos/:projetoId/integrantes", projeto.HandleAdicionarIntegrante)
	r.Handle("DELETE", "/projetos/:projetoId/integrantes", projeto.HandleRemoverIntegrante)

	// Próximas rotas do README do frontend entram aqui conforme forem
	// implementadas: /usuarios, /atividades, /materiais, etc. Cada
	// domínio novo ganha seu próprio pacote em internal/.
}

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return r.Dispatch(ctx, req)
}

func main() {
	lambda.Start(handler)
}