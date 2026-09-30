package common

import "github.com/aws/aws-lambda-go/events"

// Perfil é a única fonte de verdade dos papéis do sistema — antes disso,
// `auth` e `usuario` declaravam as mesmas três constantes de forma
// independente, e cada handler comparava a string do claim à mão.
//
// ADMIN gerencia o núcleo acadêmico (Programa, Projeto, matrícula,
// orientador) — antigo COORDENADOR. ORIENTADOR (antigo PROFESSOR) ainda não
// tem ação própria, mas é o papel que vai receber as próximas permissões
// (ex: definir campos de atividade). ALUNO nunca tem ação administrativa.
type Perfil string

const (
	Admin      Perfil = "ADMIN"
	Orientador Perfil = "ORIENTADOR"
	Aluno      Perfil = "ALUNO"
)

// PerfilPermitido checa o perfil da requisição contra um ou mais perfis
// aceitos — substitui toda comparação manual de string
// (`PerfilDaRequisicao(req) != "ADMIN"`) e qualquer helper local de grupo
// (ex: "equipe acadêmica" = Admin ou Orientador), que antes vivia duplicado
// em mais de um pacote.
func PerfilPermitido(req events.APIGatewayProxyRequest, permitidos ...Perfil) bool {
	atual := Perfil(PerfilDaRequisicao(req))
	for _, p := range permitidos {
		if atual == p {
			return true
		}
	}
	return false
}
