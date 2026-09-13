package projeto

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/programa"
)

func HandleCriar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if common.PerfilDaRequisicao(req) != "COORDENADOR" {
		return common.Erro(403, "acesso restrito a coordenadores"), nil
	}

	programaID := req.PathParameters["programaId"]

	existe, err := programa.Existe(ctx, programaID)
	if err != nil {
		return common.Erro(500, "falha ao validar programa"), nil
	}
	if !existe {
		return common.Erro(404, "programa não encontrado"), nil
	}

	var novo NovoProjeto
	if err := json.Unmarshal([]byte(req.Body), &novo); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if novo.Nome == "" {
		return common.Erro(400, "nome é obrigatório"), nil
	}

	p, err := salvar(ctx, programaID, novo)
	if err != nil {
		return common.Erro(500, "falha ao criar projeto"), nil
	}

	return common.JSON(201, p), nil
}

func HandleListarPorPrograma(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	programaID := req.PathParameters["programaId"]

	projetos, err := listarPorPrograma(ctx, programaID)
	if err != nil {
		return common.Erro(500, "falha ao listar projetos"), nil
	}
	return common.JSON(200, projetos), nil
}

func HandleAssociarOrientador(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if common.PerfilDaRequisicao(req) != "COORDENADOR" {
		return common.Erro(403, "acesso restrito a coordenadores"), nil
	}

	projetoID := req.PathParameters["projetoId"]

	var body AssociarOrientador
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if body.OrientadorID == "" {
		return common.Erro(400, "orientadorId é obrigatório"), nil
	}

	if err := associarOrientador(ctx, projetoID, body.OrientadorID); err != nil {
		return common.Erro(404, "projeto não encontrado"), nil
	}

	return common.JSON(200, map[string]string{"mensagem": "orientador associado"}), nil
}

// HandleAdicionarIntegrante implementa PUT /projetos/:projetoId/integrantes.
func HandleAdicionarIntegrante(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if common.PerfilDaRequisicao(req) != "COORDENADOR" {
		return common.Erro(403, "acesso restrito a coordenadores"), nil
	}

	projetoID := req.PathParameters["projetoId"]

	var body AssociarAluno
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if body.RGM == "" {
		return common.Erro(400, "rgm é obrigatório"), nil
	}

	if err := adicionarIntegrante(ctx, projetoID, body.RGM); err != nil {
		return common.Erro(404, "projeto não encontrado"), nil
	}

	return common.JSON(200, map[string]string{"mensagem": "aluno associado ao projeto"}), nil
}

// HandleRemoverIntegrante implementa DELETE /projetos/:projetoId/integrantes.
func HandleRemoverIntegrante(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if common.PerfilDaRequisicao(req) != "COORDENADOR" {
		return common.Erro(403, "acesso restrito a coordenadores"), nil
	}

	projetoID := req.PathParameters["projetoId"]

	var body AssociarAluno
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if body.RGM == "" {
		return common.Erro(400, "rgm é obrigatório"), nil
	}

	if err := removerIntegrante(ctx, projetoID, body.RGM); err != nil {
		return common.Erro(404, "projeto não encontrado"), nil
	}

	return common.JSON(200, map[string]string{"mensagem": "aluno removido do projeto"}), nil
}