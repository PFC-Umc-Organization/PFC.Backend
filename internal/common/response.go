// Package common tem utilitários compartilhados entre os domínios do
// backend (auth, e futuramente atividade, material, etc.) — hoje só
// serialização de resposta, pra nenhum handler duplicar os headers de CORS.
package common

import (
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
)

// headersPadrao vai em toda resposta.
//
// As de CORS (Access-Control-*) libram qualquer origem por enquanto — em
// produção, trocar "*" pelo domínio real do CloudFront. Na prática a API
// Gateway HTTP API ignora esses headers vindos da integração e aplica só o
// que está configurado no cors_configuration do Terraform, então isto aqui
// não tem efeito real hoje — mantido só pra chamada direta fora do API
// Gateway (harness/dev).
//
// Cache-Control é o que importa de verdade: sem ele, o navegador pode
// servir uma resposta antiga de GET em cache em vez de bater na rede de
// novo — o efeito prático é a tela parecer "não atualizar" depois de um
// PUT/POST, mesmo com o dado já persistido certo no DynamoDB.
var headersPadrao = map[string]string{
	"Access-Control-Allow-Origin":  "*",
	"Access-Control-Allow-Headers": "Content-Type,Authorization",
	"Access-Control-Allow-Methods": "GET,POST,PUT,DELETE,OPTIONS",
	"Content-Type":                 "application/json",
	"Cache-Control":                "no-store",
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
		Headers:    headersPadrao,
		Body:       string(b),
	}
}

// Erro é um atalho pra respostas de erro no formato { "erro": "..." }, que
// é o que o frontend espera (olhar o tratamento de erro nos *MockService).
func Erro(status int, mensagem string) events.APIGatewayProxyResponse {
	return JSON(status, map[string]string{"erro": mensagem})
}
