package common

import (
	"strings"

	"github.com/aws/aws-lambda-go/events"
)


func PerfilDaRequisicao(req events.APIGatewayProxyRequest) string {
return claimString(req, "custom:perfil")
}

// SubDaRequisicao devolve o id (sub) do usuário no Cognito.
func SubDaRequisicao(req events.APIGatewayProxyRequest) string {
	return claimString(req, "sub")
}

// NomeDaRequisicao devolve o nome cadastrado no Cognito (atributo name).
func NomeDaRequisicao(req events.APIGatewayProxyRequest) string {
	return claimString(req, "name")
}

// RGMDaRequisicao extrai o RGM do e-mail do aluno. O e-mail de aluno é
// sempre <rgm>@alunos.umc.br — o Pre Sign-up Lambda barra qualquer outro
// formato —, então a parte local é o RGM. Pra professor/coordenador o valor
// não corresponde a RGM nenhum, e é assim que deve ser.
func RGMDaRequisicao(req events.APIGatewayProxyRequest) string {
	return RGMDoEmail(claimString(req, "email"))
}

// RGMDoEmail devolve a parte local do e-mail (<rgm>@alunos.umc.br). Só faz
// sentido pra conta de ALUNO — quem chama decide pelo perfil.
func RGMDoEmail(email string) string {
	local, _, ok := strings.Cut(email, "@")
	if !ok {
		return ""
	}
	return local
}


func claimString(req events.APIGatewayProxyRequest, chave string) string {
authorizer := req.RequestContext.Authorizer
if authorizer == nil {
  return ""
}

// Payload 2.0: Authorizer["jwt"]["claims"]
if jwt, ok := authorizer["jwt"].(map[string]interface{}); ok {
  if claims, ok := jwt["claims"].(map[string]interface{}); ok {
	if v, ok := claims[chave].(string); ok {
	  return v
	}
  }
}

// Payload 1.0: Authorizer["claims"]
if claims, ok := authorizer["claims"].(map[string]interface{}); ok {
  if v, ok := claims[chave].(string); ok {
	return v
  }
}

return ""
}
