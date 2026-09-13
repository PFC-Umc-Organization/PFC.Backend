package programa

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// HandleCriar implementa POST /programas. Só coordenador cria programa.
func HandleCriar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if common.PerfilDaRequisicao(req) != "COORDENADOR" {
		return common.Erro(403, "acesso restrito a coordenadores"), nil
	}

	var novo NovoPrograma
	if err := json.Unmarshal([]byte(req.Body), &novo); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if novo.CursoID == "" {
		return common.Erro(400, "cursoId é obrigatório"), nil
	}

	p, err := salvar(ctx, novo)
	if err != nil {
		return common.Erro(500, "falha ao criar programa"), nil
	}

	return common.JSON(201, p), nil
}

// HandleListar implementa GET /programas. Aberto a qualquer usuário autenticado.
func HandleListar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	programas, err := listar(ctx)
	if err != nil {
		return common.Erro(500, "falha ao listar programas"), nil
	}
	return common.JSON(200, programas), nil
}

// HandleAtualizar implementa PUT /programas/:programaId.
func HandleAtualizar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if common.PerfilDaRequisicao(req) != "COORDENADOR" {
		return common.Erro(403, "acesso restrito a coordenadores"), nil
	}

	id := req.PathParameters["programaId"]

	var dados AtualizarPrograma
	if err := json.Unmarshal([]byte(req.Body), &dados); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if dados.CursoID == "" {
		return common.Erro(400, "cursoId é obrigatório"), nil
	}

	if err := atualizar(ctx, id, dados); err != nil {
		return common.Erro(404, "programa não encontrado"), nil
	}

	return common.JSON(200, map[string]string{"mensagem": "programa atualizado"}), nil
}

// HandleDeletar implementa DELETE /programas/:programaId.
func HandleDeletar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if common.PerfilDaRequisicao(req) != "COORDENADOR" {
		return common.Erro(403, "acesso restrito a coordenadores"), nil
	}

	id := req.PathParameters["programaId"]

	if err := deletar(ctx, id); err != nil {
		if err == errProgramaComProjetos {
			return common.Erro(409, "programa possui projetos vinculados, não pode ser removido"), nil
		}
		return common.Erro(404, "programa não encontrado"), nil
	}

	return common.JSON(200, map[string]string{"mensagem": "programa removido"}), nil
}