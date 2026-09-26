package projeto

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/matricula"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/programa"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/usuario"
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


func HandleRemoverOrientador(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if common.PerfilDaRequisicao(req) != "COORDENADOR" {
		return common.Erro(403, "acesso restrito a coordenadores"), nil
	}

	projetoID := req.PathParameters["projetoId"]

	if err := removerOrientador(ctx, projetoID); err != nil {
		return common.Erro(404, "projeto não encontrado"), nil
	}

	return common.JSON(200, map[string]string{"mensagem": "orientador removido"}), nil
}


func HandleAtualizar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if common.PerfilDaRequisicao(req) != "COORDENADOR" {
		return common.Erro(403, "acesso restrito a coordenadores"), nil
	}

	projetoID := req.PathParameters["projetoId"]

	var dados AtualizarProjeto
	if err := json.Unmarshal([]byte(req.Body), &dados); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if dados.Nome == "" {
		return common.Erro(400, "nome é obrigatório"), nil
	}

	if err := atualizarProjeto(ctx, projetoID, dados); err != nil {
		return common.Erro(404, "projeto não encontrado"), nil
	}

	return common.JSON(200, map[string]string{"mensagem": "projeto atualizado"}), nil
}

func HandleDeletar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if common.PerfilDaRequisicao(req) != "COORDENADOR" {
		return common.Erro(403, "acesso restrito a coordenadores"), nil
	}

	projetoID := req.PathParameters["projetoId"]

	if err := deletarProjeto(ctx, projetoID); err != nil {
		return common.Erro(404, "projeto não encontrado"), nil
	}

	return common.JSON(200, map[string]string{"mensagem": "projeto removido"}), nil
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

	matriculado, err := matricula.Existe(ctx, body.RGM)
	if err != nil {
		return common.Erro(500, "falha ao validar matrícula"), nil
	}
	if !matriculado {
		// Fora da allowlist, mas pode já ter conta de aluno (criada antes
		// da lista, ou RGM removido dela depois do cadastro) — conta
		// existente é aluno legítimo.
		temConta, err := usuario.AlunoExiste(ctx, body.RGM)
		if err != nil {
			log.Printf("PUT /projetos/%s/integrantes: %v", projetoID, err)
			return common.Erro(500, "falha ao validar matrícula"), nil
		}
		if !temConta {
			return common.Erro(404, "RGM não está pré-autorizado e não tem conta de aluno"), nil
		}
	}

	if err := adicionarIntegrante(ctx, projetoID, body.RGM); err != nil {
		return common.Erro(404, "projeto não encontrado"), nil
	}

	return common.JSON(200, map[string]string{"mensagem": "aluno associado ao projeto"}), nil
}

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