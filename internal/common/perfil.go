package common

import (
	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

type Perfil = actor.Role

const (
	Admin      = actor.RoleAdmin
	Orientador = actor.RoleAdvisor
	Aluno      = actor.RoleStudent
)

func PerfilPermitido(req events.APIGatewayProxyRequest, permitidos ...Perfil) bool {
	return httpapi.ActorOf(req).HasRole(permitidos...)
}
