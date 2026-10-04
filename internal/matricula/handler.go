package matricula

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auditoria"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/curso"
)

// rgmsValidos remove espaços, descarta entradas vazias e duplicadas —
// sem isso, um RGM em branco no array grava um item "STUDENT#" fantasma
// (sem RGM de fato) e repetir o mesmo RGM no mesmo lote é trabalho em
// dobro à toa.
func rgmsValidos(rgms []string) []string {
	vistos := make(map[string]bool, len(rgms))
	validos := make([]string, 0, len(rgms))
	for _, rgm := range rgms {
		rgm = strings.TrimSpace(rgm)
		if rgm == "" || vistos[rgm] {
			continue
		}
		vistos[rgm] = true
		validos = append(validos, rgm)
	}
	return validos
}


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
	turmaID := strings.TrimSpace(body.TurmaID)
	if turmaID == "" {
		return common.Erro(400, "turmaId é obrigatório"), nil
	}
	cursoExiste, err := curso.Existe(ctx, turmaID)
	if err != nil {
		return common.Erro(500, "falha ao validar turma"), nil
	}
	if !cursoExiste {
		return common.Erro(404, "turma não encontrada"), nil
	}

	rgms := rgmsValidos(body.RGMs)
	if len(rgms) == 0 {
		return common.Erro(400, "informe ao menos um RGM"), nil
	}

	falhas := gravarRGMs(ctx, rgms, turmaID)

	evento := auditoria.Evento{
		Acao: "matricula.provisionado", Resultado: "sucesso",
		Detalhes: map[string]any{"rgms": rgms, "turmaId": turmaID, "processados": len(rgms) - len(falhas)},
	}
	if len(falhas) > 0 {
		evento.Resultado = "falha"
		evento.Motivo = fmt.Sprintf("%d de %d RGMs falharam", len(falhas), len(rgms))
	}
	auditoria.Registrar(ctx, evento, req)

	return common.JSON(200, RGMResponse{
		Processados: len(rgms) - len(falhas),
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
	rgms := rgmsValidos(body.RGMs)
	if len(rgms) == 0 {
		return common.Erro(400, "informe ao menos um RGM"), nil
	}

	falhas := removerRGMs(ctx, rgms)

	evento := auditoria.Evento{
		Acao: "matricula.removido", Resultado: "sucesso",
		Detalhes: map[string]any{"rgms": rgms, "processados": len(rgms) - len(falhas)},
	}
	if len(falhas) > 0 {
		evento.Resultado = "falha"
		evento.Motivo = fmt.Sprintf("%d de %d RGMs falharam", len(falhas), len(rgms))
	}
	auditoria.Registrar(ctx, evento, req)

	return common.JSON(200, RGMResponse{
		Processados: len(rgms) - len(falhas),
		Falhas:      falhas,
	}), nil
}
