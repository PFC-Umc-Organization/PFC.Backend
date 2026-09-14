package common

import "github.com/aws/aws-lambda-go/events"


func PerfilDaRequisicao(req events.APIGatewayProxyRequest) string {
return claimString(req, "custom:perfil")
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
