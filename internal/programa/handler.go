package programa

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auditoria"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/curso"
)


func HandleCriar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !common.PerfilPermitido(req, common.Admin) {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "programa.criado", Resultado: "falha", Motivo: "acesso restrito a administradores",
		}, req)
		return common.Erro(403, "acesso restrito a administradores"), nil
	}

	var novo NovoPrograma
	if err := json.Unmarshal([]byte(req.Body), &novo); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if novo.CursoID == "" {
		return common.Erro(400, "cursoId é obrigatório"), nil
	}

	cursoExiste, err := curso.Existe(ctx, novo.CursoID)
	if err != nil {
		return common.Erro(500, "falha ao criar programa"), nil
	}
	if !cursoExiste {
		return common.Erro(404, "curso não encontrado"), nil
	}

	duplicado, err := existeParaCurso(ctx, "", novo.CursoID)
	if err != nil {
		return common.Erro(500, "falha ao criar programa"), nil
	}
	if duplicado {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "programa.criado", Resultado: "falha", Motivo: "programa já existe para este curso",
			Detalhes: map[string]any{"cursoId": novo.CursoID},
		}, req)
		return common.Erro(409, "este curso já possui um programa de PFC"), nil
	}

	p, err := salvar(ctx, novo)
	if err != nil {
		return common.Erro(500, "falha ao criar programa"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "programa.criado", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "programa", ID: p.ID},
		Detalhes: map[string]any{"cursoId": p.CursoID},
	}, req)

	return common.JSON(201, p), nil
}


func HandleListar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	programas, err := listar(ctx)
	if err != nil {
		return common.Erro(500, "falha ao listar programas"), nil
	}
	return common.JSON(200, programas), nil
}


func HandleAtualizar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !common.PerfilPermitido(req, common.Admin) {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "programa.atualizado", Resultado: "falha", Motivo: "acesso restrito a administradores",
		}, req)
		return common.Erro(403, "acesso restrito a administradores"), nil
	}

	id := req.PathParameters["programaId"]

	var dados AtualizarPrograma
	if err := json.Unmarshal([]byte(req.Body), &dados); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if dados.CursoID == "" {
		return common.Erro(400, "cursoId é obrigatório"), nil
	}

	cursoExiste, err := curso.Existe(ctx, dados.CursoID)
	if err != nil {
		return common.Erro(500, "falha ao atualizar programa"), nil
	}
	if !cursoExiste {
		return common.Erro(404, "curso não encontrado"), nil
	}

	duplicado, err := existeParaCurso(ctx, id, dados.CursoID)
	if err != nil {
		return common.Erro(500, "falha ao atualizar programa"), nil
	}
	if duplicado {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "programa.atualizado", Resultado: "falha", Motivo: "programa já existe para este curso",
			Recurso:  &auditoria.Recurso{Tipo: "programa", ID: id},
			Detalhes: map[string]any{"cursoId": dados.CursoID},
		}, req)
		return common.Erro(409, "este curso já possui um programa de PFC"), nil
	}

	if err := atualizar(ctx, id, dados); err != nil {
		return common.Erro(404, "programa não encontrado"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "programa.atualizado", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "programa", ID: id},
		Detalhes: map[string]any{"cursoId": dados.CursoID},
	}, req)

	return common.JSON(200, map[string]string{"mensagem": "programa atualizado"}), nil
}


func HandleDeletar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !common.PerfilPermitido(req, common.Admin) {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "programa.removido", Resultado: "falha", Motivo: "acesso restrito a administradores",
		}, req)
		return common.Erro(403, "acesso restrito a administradores"), nil
	}

	id := req.PathParameters["programaId"]

	if err := deletar(ctx, id); err != nil {
		if err == errProgramaComProjetos {
			auditoria.Registrar(ctx, auditoria.Evento{
				Acao: "programa.removido", Resultado: "falha", Motivo: "programa possui projetos vinculados",
				Recurso: &auditoria.Recurso{Tipo: "programa", ID: id},
			}, req)
			return common.Erro(409, "programa possui projetos vinculados, não pode ser removido"), nil
		}
		return common.Erro(404, "programa não encontrado"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "programa.removido", Resultado: "sucesso",
		Recurso: &auditoria.Recurso{Tipo: "programa", ID: id},
	}, req)

	return common.JSON(200, map[string]string{"mensagem": "programa removido"}), nil
}