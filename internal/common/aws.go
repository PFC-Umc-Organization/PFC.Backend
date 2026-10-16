package common

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// Configuração lida do ambiente da Lambda.
var (
	Tabela = os.Getenv("TABLE_NAME")
	Indice = os.Getenv("GSI_NAME")
)

// variaveisObrigatorias são as que, faltando, quebram toda requisição.
var variaveisObrigatorias = []string{"TABLE_NAME", "GSI_NAME", "COGNITO_CLIENT_ID", "COGNITO_USER_POOL_ID"}

// AmbienteIncompleto devolve as variáveis obrigatórias que não estão
// definidas. main recusa subir sem elas: é melhor a Lambda falhar no init,
// com a causa no log, do que responder 500 em toda chamada.
func AmbienteIncompleto() []string {
	var faltando []string
	for _, v := range variaveisObrigatorias {
		if os.Getenv(v) == "" {
			faltando = append(faltando, v)
		}
	}
	return faltando
}

// Os clientes AWS são únicos no processo. Antes cada pacote carregava a
// própria config em init(): oito cargas no cold start e um pool de conexões
// por pacote — uma requisição que passava por projeto, matricula e programa
// abria três conexões TLS com o DynamoDB em vez de reaproveitar uma.
var configAWS = sync.OnceValue(func() aws.Config {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(fmt.Sprintf("falha ao carregar config AWS: %v", err))
	}
	return cfg
})

// DynamoDB devolve o cliente compartilhado do DynamoDB.
var DynamoDB = sync.OnceValue(func() *dynamodb.Client {
	return dynamodb.NewFromConfig(configAWS())
})

// Cognito devolve o cliente compartilhado do Cognito.
var Cognito = sync.OnceValue(func() *cognitoidentityprovider.Client {
	return cognitoidentityprovider.NewFromConfig(configAWS())
})
