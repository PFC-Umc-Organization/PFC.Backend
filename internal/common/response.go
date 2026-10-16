package common

import (
	"log"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

func JSON(status int, body any) events.APIGatewayProxyResponse {
	return httpapi.ToLambda(httpapi.JSON(status, body))
}

func Erro(status int, mensagem string) events.APIGatewayProxyResponse {
	return httpapi.ToLambda(httpapi.Error(status, mensagem))
}

// ErroDeEscrita traduz o erro de uma escrita condicionada a
// attribute_exists(PK): condição falhou = o item não existe (404); qualquer
// outro erro é falha de infraestrutura (500), logada no CloudWatch.
func ErroDeEscrita(err error, naoEncontrado string) events.APIGatewayProxyResponse {
	if FalhouCondicao(err) {
		return Erro(404, naoEncontrado)
	}
	log.Printf("escrita no DynamoDB: %v", err)
	return Erro(500, "falha ao salvar — tente novamente em instantes")
}
