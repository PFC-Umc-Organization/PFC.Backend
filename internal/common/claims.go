package common

import "github.com/aws/aws-lambda-go/events"

// PerfilDaRequisicao lê o atributo customizado custom:perfil que o
// Cognito injeta no contexto do Authorizer (via REST API +
// COGNITO_USER_POOLS). Retorna string vazia se a rota não tiver
// Authorizer (pública) ou se o claim não existir.
//
// Por que atributo customizado em vez de grupo Cognito: perfil é dado do
// domínio de negócio (quem essa pessoa É — aluno, professor,
// coordenador), não uma decisão de infraestrutura como "quem pode gravar
// no DynamoDB". Setado explicitamente na criação da conta (AdminCreateUser
// com UserAttributes custom:perfil=PROFESSOR, por exemplo) — nunca
// aceito de cadastro público, pelo mesmo motivo que /auth/registrar
// ignora o perfil enviado pelo cliente.
func PerfilDaRequisicao(req events.APIGatewayProxyRequest) string {
	// Rota autenticada por um JWT Authorizer do API Gateway HTTP API (não
	// um Cognito User Pools Authorizer de REST API) — por isso os claims
	// vêm aninhados em Authorizer["jwt"]["claims"], não direto em
	// Authorizer["claims"] como seria numa REST API.
	jwt, ok := req.RequestContext.Authorizer["jwt"].(map[string]interface{})
	if !ok {
		return ""
	}
	claims, ok := jwt["claims"].(map[string]interface{})
	if !ok {
		return ""
	}
	perfil, _ := claims["custom:perfil"].(string)
	return perfil
}
