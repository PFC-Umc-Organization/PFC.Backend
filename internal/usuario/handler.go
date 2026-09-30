package usuario

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// HandleListar implementa GET /usuarios.
//
// Qualquer autenticado pode listar: o aluno precisa dos nomes pra ver os
// colegas de grupo e o orientador (tela Meu PFC). Mas e-mail é dado pessoal
// e só a equipe acadêmica recebe — pro aluno ele sai da resposta.
func HandleListar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	usuarios, err := listarContas(ctx)
	if err != nil {
		// Vai pro CloudWatch — o cliente só recebe a mensagem genérica.
		log.Printf("GET /usuarios: %v", err)
		return common.Erro(500, "falha ao listar usuários"), nil
	}

	if !common.PerfilPermitido(req, common.Admin, common.Orientador) {
		for i := range usuarios {
			usuarios[i].Email = ""
		}
	}
	return common.JSON(200, usuarios), nil
}
