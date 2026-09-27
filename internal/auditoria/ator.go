package auditoria

import (
	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// AtorDaRequisicao monta o Ator a partir dos claims do JWT e do IP de
// origem. Fica exportado porque quem monta o Evento pode querer inspecionar
// o ator antes de decidir Resultado/Motivo (ex: não é o caso hoje, mas evita
// precisar de outra função se surgir).
func AtorDaRequisicao(req events.APIGatewayProxyRequest) Ator {
	return Ator{
		Sub:    common.SubDaRequisicao(req),
		Perfil: common.PerfilDaRequisicao(req),
		Email:  common.EmailDaRequisicao(req),
		IP:     req.RequestContext.Identity.SourceIP,
	}
}
