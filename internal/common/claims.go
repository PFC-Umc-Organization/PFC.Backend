// PerfilDaRequisicao lê o atributo customizado custom:perfil que o Cognito
// injeta no contexto do JWT Authorizer do HTTP API. Retorna string vazia se
// a rota não tiver Authorizer (pública) ou se o claim não existir.
//
// Por que atributo customizado em vez de grupo Cognito: perfil é dado do
// domínio de negócio (quem essa pessoa É — aluno, professor, coordenador),
// não uma decisão de infraestrutura como "quem pode gravar no DynamoDB".
// Setado explicitamente na criação da conta (AdminCreateUser com
// UserAttributes custom:perfil=PROFESSOR, por exemplo) — nunca aceito de
// cadastro público, pelo mesmo motivo que /auth/registrar ignora o perfil
// enviado pelo cliente.
//
// Onde os claims ficam depende do PayloadFormatVersion da integração:
//   - 2.0: RequestContext.Authorizer["jwt"]["claims"]
//   - 1.0: RequestContext.Authorizer["claims"]
// A integração hoje está em 1.0, mas lemos os dois pra não quebrar se
// mudar. Ver PayloadFormatVersion no módulo api-gateway do Terraform.
func PerfilDaRequisicao(req events.APIGatewayProxyRequest) string {
return claimString(req, "custom:perfil")
}

// claimString extrai um claim do contexto do Authorizer, tolerando os dois
// formatos de payload do API Gateway (2.0 aninha em "jwt.claims", 1.0 põe
// direto em "claims").
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
