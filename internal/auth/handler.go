package auth

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// HandleLogin implementa POST /auth/login.
func HandleLogin(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var cred Credenciais
	if err := json.Unmarshal([]byte(req.Body), &cred); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}

	idToken, err := cognitoLogin(ctx, cred)
	if err != nil {
		// Não expõe o erro cru do Cognito pro cliente — mensagem genérica,
		// consistente com o que o front já trata hoje no mock
		// ("E-mail ou senha inválidos.").
		return common.Erro(401, "e-mail ou senha inválidos"), nil
	}

	claims, err := claimsDoIdToken(idToken)
	if err != nil {
		return common.Erro(500, "falha ao processar autenticação"), nil
	}

	perfil := perfilDosClaims(claims)
	usuario := usuarioDosClaims(claims, perfil)

	return common.JSON(200, RespostaAuth{Usuario: usuario, Token: idToken}), nil
}

// HandleRegistrar implementa POST /auth/registrar.
func HandleRegistrar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var novo NovoUsuario
	if err := json.Unmarshal([]byte(req.Body), &novo); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}

	// IMPORTANTE: o campo `novo.Perfil` vem do cliente e NUNCA é usado pra
	// decidir o perfil real. Esse endpoint é público (sem autenticação) —
	// se confiássemos no valor enviado, qualquer requisição poderia se
	// autodeclarar PROFESSOR. Cadastro público sempre cria ALUNO; contas de
	// professor só existem via AdminCreateUser (fora deste endpoint).
	novo.Perfil = PerfilAluno

	if err := cognitoSignUp(ctx, novo); err != nil {
		// O Pre Sign-up Lambda do Cognito é quem efetivamente barra RGMs
		// fora da allowlist ou domínio errado — o erro que chega aqui já
		// vem dessa validação. Repassamos uma mensagem genérica por ora;
		// dá pra inspecionar o tipo do erro depois pra diferenciar "RGM
		// não matriculado" de "e-mail já cadastrado", se fizer sentido
		// pra UX.
		return common.Erro(400, "não foi possível concluir o cadastro"), nil
	}

	// SignUp não retorna token — o Cognito pode exigir confirmação por
	// e-mail antes do primeiro login (ver ConfirmSignUp). O frontend
	// precisa saber disso pra não tentar redirecionar como se o usuário
	// já estivesse autenticado.
	return common.JSON(202, map[string]string{
		"mensagem": "cadastro recebido — confirme o código enviado por e-mail antes de entrar",
	}), nil
}

// perfilDosClaims lê custom:perfil do JWT recém-emitido. Contas
// self-registradas (aluno) nunca têm esse atributo setado — por isso o
// default é ALUNO quando ausente. Contas de professor/coordenador são
// criadas via AdminCreateUser com o atributo explícito (fora deste
// endpoint, feito pelo admin/coordenador).
func perfilDosClaims(claims map[string]any) Perfil {
	if raw, ok := claims["custom:perfil"].(string); ok && raw != "" {
		return Perfil(raw)
	}
	return PerfilAluno
}
