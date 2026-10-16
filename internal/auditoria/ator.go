package auditoria

import (
	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

func actorOf(req events.APIGatewayProxyRequest) audit.Actor {
	a := httpapi.ActorOf(req)
	return audit.Actor{Sub: a.Sub, Role: string(a.Role), Email: a.Email, IP: req.RequestContext.Identity.SourceIP}
}
