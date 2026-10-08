package main

import (
	"context"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/atividade"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auth"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/curso"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/matricula"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/programa"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/projeto"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/referencia"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/router"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/usuario"
)

var r *router.Router

func init() {
	r = router.New()

	// Autenticação
	r.Handle("POST", "/auth/login", auth.HandleLogin)
	r.Handle("POST", "/auth/registrar", auth.HandleRegistrar)
	r.Handle("POST", "/auth/confirmar", auth.HandleConfirmar)
	r.Handle("POST", "/auth/reenviar-codigo", auth.HandleReenviarCodigo)

	// Usuários (contas do Cognito)
	r.Handle("GET", "/usuarios", usuario.HandleListar)
	r.Handle("POST", "/admin/usuarios", usuario.HandleCriarConta)

	// Turmas
	r.Handle("POST", "/turmas", curso.HandleCriar)
	r.Handle("GET", "/turmas", curso.HandleListar)
	r.Handle("PUT", "/turmas/:turmaId", curso.HandleAtualizar)
	r.Handle("DELETE", "/turmas/:turmaId", curso.HandleDeletar)

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
	r.Handle("POST", "/meu-pfc", projeto.HandleCriarDoAluno)
	r.Handle("POST", "/programas/:programaId/projetos", projeto.HandleCriar)
	r.Handle("GET", "/programas/:programaId/projetos", projeto.HandleListarPorPrograma)
	r.Handle("PUT", "/projetos/:projetoId", projeto.HandleAtualizar)
	r.Handle("DELETE", "/projetos/:projetoId", projeto.HandleDeletar)
	r.Handle("PUT", "/projetos/:projetoId/orientador", projeto.HandleAssociarOrientador)
	r.Handle("DELETE", "/projetos/:projetoId/orientador", projeto.HandleRemoverOrientador)
	r.Handle("PUT", "/projetos/:projetoId/integrantes", projeto.HandleAdicionarIntegrante)
	r.Handle("DELETE", "/projetos/:projetoId/integrantes", projeto.HandleRemoverIntegrante)

	// Atividades, campos de entrega e entregas
	r.Handle("GET", "/atividades", atividade.HandleListar)
	r.Handle("POST", "/atividades", atividade.HandleCriar)
	r.Handle("PUT", "/atividades/:atividadeId", atividade.HandleAtualizar)
	r.Handle("DELETE", "/atividades/:atividadeId", atividade.HandleDeletar)
	r.Handle("POST", "/atividades/:atividadeId/campos", atividade.HandleAdicionarCampo)
	r.Handle("DELETE", "/atividades/:atividadeId/campos/:campoId", atividade.HandleRemoverCampo)
	r.Handle("GET", "/atividades/:atividadeId/entregas", atividade.HandleListarEntregasDaAtividade)
	r.Handle("GET", "/projetos/:projetoId/entregas", atividade.HandleListarEntregasDoProjeto)
	r.Handle("PUT", "/projetos/:projetoId/entregas/:atividadeId", atividade.HandleEntregar)
	r.Handle("DELETE", "/projetos/:projetoId/entregas/:atividadeId", atividade.HandleRemoverEntrega)

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