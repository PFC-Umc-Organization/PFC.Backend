package main

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/app"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

func main() {
	if missing := common.AmbienteIncompleto(); len(missing) > 0 {
		log.Fatalf("variáveis de ambiente obrigatórias ausentes: %v", missing)
	}
	common.DynamoDB()
	common.Cognito()

	r := app.Router()
	lambda.Start(func(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		return r.Dispatch(ctx, req)
	})
}
