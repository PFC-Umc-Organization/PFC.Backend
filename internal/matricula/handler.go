package matricula

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// ehCoordenador reaproveita o helper compartilhado — a allowlist de RGMs
// é gestão do programa, então cai na mesma regra de autorização que
// programas/projetos.
func ehCoordenador(req events.APIGatewayProxyRequest) bool {
	return common.PerfilDaRequisicao(req) == "COORDENADOR"
}

// HandleProvisionar implementa POST /admin/students.
func HandleProvisionar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !ehCoordenador(req) {
		return common.Erro(403, "acesso restrito a coordenadores"), nil
	}

	var body RGMRequest
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if len(body.RGMs) == 0 {
		return common.Erro(400, "informe ao menos um RGM"), nil
	}

	falhas := gravarRGMs(ctx, body.RGMs)

	return common.JSON(200, RGMResponse{
		Processados: len(body.RGMs) - len(falhas),
		Falhas:      falhas,
	}), nil
}

// HandleRemover implementa DELETE /admin/students.
func HandleRemover(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !ehCoordenador(req) {
		return common.Erro(403, "acesso restrito a coordenadores"), nil
	}

	var body RGMRequest
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if len(body.RGMs) == 0 {
		return common.Erro(400, "informe ao menos um RGM"), nil
	}

	falhas := removerRGMs(ctx, body.RGMs)

	return common.JSON(200, RGMResponse{
		Processados: len(body.RGMs) - len(falhas),
		Falhas:      falhas,
	}), nil
}
