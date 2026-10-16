package common

import (
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"
)

var headersPadrao = map[string]string{
	"Access-Control-Allow-Origin":  "*",
	"Access-Control-Allow-Headers": "Content-Type,Authorization",
	"Access-Control-Allow-Methods": "GET,POST,PUT,DELETE,OPTIONS",
	"Content-Type":                 "application/json",
	"Cache-Control":                "no-store",
}

func JSON(status int, body any) events.APIGatewayProxyResponse {
	b, err := json.Marshal(body)
	if err != nil {
		return Erro(500, "falha ao serializar resposta")
	}
	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Headers:    headersPadrao,
		Body:       string(b),
	}
}

func Erro(status int, mensagem string) events.APIGatewayProxyResponse {
	return JSON(status, map[string]string{"erro": mensagem})
}

// ErroDeEscrita traduz o erro de uma escrita condicionada a
// attribute_exists(PK): condição falhou = o item não existe (404); qualquer
// outro erro é falha de infraestrutura (500), logada no CloudWatch — antes
// todo erro virava 404 e um throttling aparecia como "não encontrado".
func ErroDeEscrita(err error, naoEncontrado string) events.APIGatewayProxyResponse {
	if FalhouCondicao(err) {
		return Erro(404, naoEncontrado)
	}
	log.Printf("escrita no DynamoDB: %v", err)
	return Erro(500, "falha ao salvar — tente novamente em instantes")
}
