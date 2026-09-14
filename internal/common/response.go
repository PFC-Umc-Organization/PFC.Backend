
package common

import (
	"encoding/json"

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
