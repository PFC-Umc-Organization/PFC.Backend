package httpapi

import (
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
)

// ---------------------------------------------------------------------------
// Requisição
// ---------------------------------------------------------------------------

const msgInvalidBody = "Não foi possível ler os dados enviados. Atualize a página e tente novamente."

// Requisição
type Request struct {
	Method    string
	Path      string
	Body      string
	Actor     actor.Actor
	SourceIP  string
	RequestID string

	params map[string]string
	query  map[string]string
}

// Montagem
func newRequest(ev events.APIGatewayProxyRequest, params map[string]string) *Request {
	return &Request{
		Method:    ev.HTTPMethod,
		Path:      ev.Path,
		Body:      ev.Body,
		Actor:     actorOf(ev),
		SourceIP:  ev.RequestContext.Identity.SourceIP,
		RequestID: ev.RequestContext.RequestID,
		params:    params,
		query:     ev.QueryStringParameters,
	}
}

// Consulta
func (r *Request) Param(name string) string { return r.params[name] }

// Consulta
func (r *Request) Query(name string) string { return r.query[name] }

// Leitura
func (r *Request) Decode(v any) error {
	if err := json.Unmarshal([]byte(r.Body), v); err != nil {
		return apperr.Invalid(msgInvalidBody)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Ator
// ---------------------------------------------------------------------------

// Extração
func actorOf(ev events.APIGatewayProxyRequest) actor.Actor {
	claims := claimsOf(ev.RequestContext.Authorizer)
	str := func(k string) string { s, _ := claims[k].(string); return s }
	return actor.Actor{
		Sub:   str("sub"),
		Name:  actor.DisplayName(str("name"), str("email")),
		Email: str("email"),
		Role:  actor.Role(str("custom:perfil")),
	}
}

// Leitura
func claimsOf(authorizer map[string]any) map[string]any {
	if jwt, ok := authorizer["jwt"].(map[string]any); ok {
		if claims, ok := jwt["claims"].(map[string]any); ok {
			return claims
		}
	}
	claims, _ := authorizer["claims"].(map[string]any)
	return claims
}
