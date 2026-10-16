package common

import (
	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

func PerfilDaRequisicao(req events.APIGatewayProxyRequest) string {
	return string(httpapi.ActorOf(req).Role)
}

func SubDaRequisicao(req events.APIGatewayProxyRequest) string { return httpapi.ActorOf(req).Sub }

func NomeDaRequisicao(req events.APIGatewayProxyRequest) string { return httpapi.ActorOf(req).Name }

func EmailDaRequisicao(req events.APIGatewayProxyRequest) string { return httpapi.ActorOf(req).Email }

func RGMDaRequisicao(req events.APIGatewayProxyRequest) string { return httpapi.ActorOf(req).RGM() }

func RGMDoEmail(email string) string { return actor.RGMFromEmail(email) }
