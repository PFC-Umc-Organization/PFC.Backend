// Package auditoria registra, em formato estruturado, quem fez o quê no
// sistema — login, criação/edição/remoção de Programa, Projeto, matrícula e
// referência. É o que a política de privacidade do frontend já promete ao
// usuário ("logs de auditoria... via AWS CloudWatch"); este pacote é onde
// essa promessa passa a ser cumprida de fato.
//
// v1: cada evento é uma linha JSON emitida no mesmo Log Group da Lambda
// (via log/slog), marcada com tipo=auditoria para diferenciar de log comum
// de debug/erro e ser filtrável no CloudWatch Logs Insights. Não há
// armazenamento durável fora do CloudWatch ainda — isso é um próximo passo
// deliberadamente fora desta primeira versão.
package auditoria

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/events"
)

// logger é variável de pacote pra os testes substituírem a saída por um
// buffer, em vez de os.Stdout.
var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

// Ator identifica quem fez a ação, extraído do JWT (via Cognito Authorizer)
// e do IP de origem da requisição.
type Ator struct {
	Sub    string `json:"sub,omitempty"`
	Perfil string `json:"perfil,omitempty"`
	Email  string `json:"email,omitempty"`
	IP     string `json:"ip,omitempty"`
}

// Recurso identifica o que foi afetado pela ação.
type Recurso struct {
	Tipo string `json:"tipo"`
	ID   string `json:"id,omitempty"`
}

// Evento é um registro de auditoria. Acao segue o padrão "dominio.acao" (ex:
// "programa.criado", "auth.login.falha"). Resultado é sempre "sucesso" ou
// "falha"; Motivo só é preenchido quando Resultado é "falha".
//
// Ator normalmente é omitido: Registrar extrai sub/perfil/email dos claims
// do JWT (via Cognito Authorizer). Rotas públicas (login, cadastro,
// confirmação) não têm Authorizer — ainda não existe sessão —, então esses
// handlers preenchem Ator explicitamente com o que sabem da própria
// tentativa (o e-mail digitado, e o perfil/sub depois de um login
// bem-sucedido).
type Evento struct {
	Acao      string
	Resultado string
	Motivo    string
	Ator      *Ator
	Recurso   *Recurso
	Detalhes  map[string]any
}

// Registrar grava o evento como uma linha JSON estruturada. Quando Ator não
// é informado, é extraído da própria requisição (claims do JWT + IP).
func Registrar(ctx context.Context, ev Evento, req events.APIGatewayProxyRequest) {
	ator := ev.Ator
	if ator == nil {
		a := AtorDaRequisicao(req)
		ator = &a
	} else if ator.IP == "" {
		ator.IP = req.RequestContext.Identity.SourceIP
	}

	atributos := []any{
		slog.String("tipo", "auditoria"),
		slog.String("acao", ev.Acao),
		slog.String("resultado", ev.Resultado),
		slog.Any("ator", ator),
	}
	if ev.Motivo != "" {
		atributos = append(atributos, slog.String("motivo", ev.Motivo))
	}
	if ev.Recurso != nil {
		atributos = append(atributos, slog.Any("recurso", ev.Recurso))
	}
	if len(ev.Detalhes) > 0 {
		atributos = append(atributos, slog.Any("detalhes", ev.Detalhes))
	}
	if id := req.RequestContext.RequestID; id != "" {
		atributos = append(atributos, slog.String("requestId", id))
	}

	logger.InfoContext(ctx, "evento de auditoria", atributos...)
}
