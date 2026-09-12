// Package common tem utilitários compartilhados entre os domínios do
// backend (auth, e futuramente atividade, material, etc.) — hoje só
// serialização de resposta, pra nenhum handler duplicar os headers de CORS.
package common

import (
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
)

// corsHeaders libera qualquer origem por enquanto (harness/dev). Em
// produção, trocar "*" pelo domínio real do CloudFront do frontend —
// deixar "*" com credenciais/cookies seria problema, mas como a auth aqui
// é via Bearer token (não cookie), o risco prático é baixo; ainda assim,
// vale restringir antes de ir pra produção de verdade.
var corsHeaders = map[string]string{
	"Access-Control-Allow-Origin":  "*",
	"Access-Control-Allow-Headers": "Content-Type,Authorization",
	"Access-Control-Allow-Methods": "GET,POST,PUT,DELETE,OPTIONS",
	"Content-Type":                 "application/json",
}

// JSON serializa qualquer valor como corpo da resposta, com os headers
// padrão já aplicados.
func JSON(status int, body any) events.APIGatewayProxyResponse {
	b, err := json.Marshal(body)
	if err != nil {
		return Erro(500, "falha ao serializar resposta")
	}
	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Headers:    corsHeaders,
		Body:       string(b),
	}
}

// Erro é um atalho pra respostas de erro no formato { "erro": "..." }, que
// é o que o frontend espera (olhar o tratamento de erro nos *MockService).
func Erro(status int, mensagem string) events.APIGatewayProxyResponse {
	return JSON(status, map[string]string{"erro": mensagem})
}
