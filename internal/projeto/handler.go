package projeto

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/programa"
)

// HandleCriar implementa POST /programas/:programaId/projetos.
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

// HandleListarPorPrograma implementa GET /programas/:programaId/projetos.
// Aberto a qualquer usuário autenticado.
func HandleListarPorPrograma(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	programaID := req.PathParameters["programaId"]

	projetos, err := listarPorPrograma(ctx, programaID)
	if err != nil {
		return common.Erro(500, "falha ao listar projetos"), nil
	}
	return common.JSON(200, projetos), nil
}

// HandleAssociarOrientador implementa PUT /projetos/:projetoId/orientador.
//
// TODO: hoje não validamos que orientadorId corresponde de fato a um
// Usuario com perfil PROFESSOR no Cognito — o handler confia no valor
// enviado pelo coordenador. Isso é diferente do caso de /auth/registrar
// (onde o RISCO era o próprio usuário se autopromover); aqui quem chama
// já é coordenador autenticado, então o risco é mais "erro de digitação"
// que "escalação de privilégio". Se fizer sentido, dá pra validar contra
// o Cognito (AdminGetUser) antes de gravar.
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
