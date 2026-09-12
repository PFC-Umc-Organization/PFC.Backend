package programa

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// HandleCriar implementa POST /programas. Só coordenador cria programa —
// é decisão de gestão acadêmica, não algo que um professor comum ou aluno
// deveria conseguir fazer.
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

// HandleListar implementa GET /programas. Aberto a qualquer usuário
// autenticado (aluno precisa ver o programa do próprio curso, por
// exemplo) — só a criação é restrita.
func HandleListar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	programas, err := listar(ctx)
	if err != nil {
		return common.Erro(500, "falha ao listar programas"), nil
	}
	return common.JSON(200, programas), nil
}
