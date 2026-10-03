package curso

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auditoria"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// HandleCriar implementa POST /turmas. Só admin cria turma.
func HandleCriar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !common.PerfilPermitido(req, common.Admin) {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "turma.criada", Resultado: "falha", Motivo: "acesso restrito a administradores",
		}, req)
		return common.Erro(403, "acesso restrito a administradores"), nil
	}

	var novo NovoCurso
	if err := json.Unmarshal([]byte(req.Body), &novo); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if novo.Nome == "" || novo.Turno == "" || novo.Periodo == "" {
		return common.Erro(400, "nome, turno e periodo são obrigatórios"), nil
	}
	if !nomeValido(novo.Nome) {
		return common.Erro(400, "curso inválido — escolha um dos cursos disponíveis"), nil
	}

	c, err := salvar(ctx, novo)
	if err != nil {
		return common.Erro(500, "falha ao criar turma"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "turma.criada", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "turma", ID: c.ID},
		Detalhes: map[string]any{"nome": c.Nome, "turno": c.Turno, "periodo": c.Periodo},
	}, req)

	return common.JSON(201, c), nil
}

// HandleListar implementa GET /turmas. Aberto a qualquer autenticado.
func HandleListar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	cursos, err := listar(ctx)
	if err != nil {
		return common.Erro(500, "falha ao listar turmas"), nil
	}
	return common.JSON(200, cursos), nil
}

// HandleAtualizar implementa PUT /turmas/:turmaId.
func HandleAtualizar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !common.PerfilPermitido(req, common.Admin) {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "turma.atualizada", Resultado: "falha", Motivo: "acesso restrito a administradores",
		}, req)
		return common.Erro(403, "acesso restrito a administradores"), nil
	}

	id := req.PathParameters["turmaId"]

	var dados AtualizarCurso
	if err := json.Unmarshal([]byte(req.Body), &dados); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if dados.Nome == "" || dados.Turno == "" || dados.Periodo == "" {
		return common.Erro(400, "nome, turno e periodo são obrigatórios"), nil
	}
	if !nomeValido(dados.Nome) {
		return common.Erro(400, "curso inválido — escolha um dos cursos disponíveis"), nil
	}

	if err := atualizar(ctx, id, dados); err != nil {
		return common.Erro(404, "turma não encontrada"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "turma.atualizada", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "turma", ID: id},
		Detalhes: map[string]any{"nome": dados.Nome, "turno": dados.Turno, "periodo": dados.Periodo},
	}, req)

	return common.JSON(200, map[string]string{"mensagem": "turma atualizada"}), nil
}

// HandleDeletar implementa DELETE /turmas/:turmaId.
func HandleDeletar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !common.PerfilPermitido(req, common.Admin) {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "turma.removida", Resultado: "falha", Motivo: "acesso restrito a administradores",
		}, req)
		return common.Erro(403, "acesso restrito a administradores"), nil
	}

	id := req.PathParameters["turmaId"]

	if err := deletar(ctx, id); err != nil {
		if err == errCursoComPrograma {
			auditoria.Registrar(ctx, auditoria.Evento{
				Acao: "turma.removida", Resultado: "falha", Motivo: "turma possui programa vinculado",
				Recurso: &auditoria.Recurso{Tipo: "turma", ID: id},
			}, req)
			return common.Erro(409, "turma possui programa vinculado, não pode ser removida"), nil
		}
		return common.Erro(404, "turma não encontrada"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "turma.removida", Resultado: "sucesso",
		Recurso: &auditoria.Recurso{Tipo: "turma", ID: id},
	}, req)

	return common.JSON(200, map[string]string{"mensagem": "turma removida"}), nil
}
