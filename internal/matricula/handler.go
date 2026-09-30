package matricula

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auditoria"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)


func HandleListar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !common.PerfilPermitido(req, common.Admin) {
		return common.Erro(403, "acesso restrito a administradores"), nil
	}

	matriculas, err := listarRGMs(ctx)
	if err != nil {
		return common.Erro(500, "falha ao listar matrículas"), nil
	}
	return common.JSON(200, matriculas), nil
}


func HandleProvisionar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !common.PerfilPermitido(req, common.Admin) {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "matricula.provisionado", Resultado: "falha", Motivo: "acesso restrito a administradores",
		}, req)
		return common.Erro(403, "acesso restrito a administradores"), nil
	}

	var body RGMRequest
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if len(body.RGMs) == 0 {
		return common.Erro(400, "informe ao menos um RGM"), nil
	}

	falhas := gravarRGMs(ctx, body.RGMs)

	evento := auditoria.Evento{
		Acao: "matricula.provisionado", Resultado: "sucesso",
		Detalhes: map[string]any{"rgms": body.RGMs, "processados": len(body.RGMs) - len(falhas)},
	}
	if len(falhas) > 0 {
		evento.Resultado = "falha"
		evento.Motivo = fmt.Sprintf("%d de %d RGMs falharam", len(falhas), len(body.RGMs))
	}
	auditoria.Registrar(ctx, evento, req)

	return common.JSON(200, RGMResponse{
		Processados: len(body.RGMs) - len(falhas),
		Falhas:      falhas,
	}), nil
}


func HandleRemover(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !common.PerfilPermitido(req, common.Admin) {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "matricula.removido", Resultado: "falha", Motivo: "acesso restrito a administradores",
		}, req)
		return common.Erro(403, "acesso restrito a administradores"), nil
	}

	var body RGMRequest
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	if len(body.RGMs) == 0 {
		return common.Erro(400, "informe ao menos um RGM"), nil
	}

	falhas := removerRGMs(ctx, body.RGMs)

	evento := auditoria.Evento{
		Acao: "matricula.removido", Resultado: "sucesso",
		Detalhes: map[string]any{"rgms": body.RGMs, "processados": len(body.RGMs) - len(falhas)},
	}
	if len(falhas) > 0 {
		evento.Resultado = "falha"
		evento.Motivo = fmt.Sprintf("%d de %d RGMs falharam", len(falhas), len(body.RGMs))
	}
	auditoria.Registrar(ctx, evento, req)

	return common.JSON(200, RGMResponse{
		Processados: len(body.RGMs) - len(falhas),
		Falhas:      falhas,
	}), nil
}
