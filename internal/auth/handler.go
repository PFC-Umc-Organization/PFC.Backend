package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auditoria"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// HandleLogin implementa POST /auth/login.
func HandleLogin(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var cred Credenciais
	if err := json.Unmarshal([]byte(req.Body), &cred); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}

	idToken, err := cognitoLogin(ctx, cred)
	var naoConfirmado *types.UserNotConfirmedException
	if errors.As(err, &naoConfirmado) {
		// Senha certa, mas a conta ainda espera o código do e-mail. O front
		// usa o 403 pra mandar o aluno pra tela de confirmação.
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "auth.login", Resultado: "falha", Motivo: "conta ainda não confirmada",
			Ator: &auditoria.Ator{Email: cred.Email},
		}, req)
		return common.Erro(403, "conta ainda não confirmada — digite o código enviado por e-mail"), nil
	}
	if err != nil {
		// Não expõe o erro cru do Cognito pro cliente — mensagem genérica,
		// consistente com o que o front já trata hoje no mock
		// ("E-mail ou senha inválidos.").
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "auth.login", Resultado: "falha", Motivo: "credenciais inválidas",
			Ator: &auditoria.Ator{Email: cred.Email},
		}, req)
		return common.Erro(401, "e-mail ou senha inválidos"), nil
	}

	claims, err := claimsDoIdToken(idToken)
	if err != nil {
		return common.Erro(500, "falha ao processar autenticação"), nil
	}

	perfil := perfilDosClaims(claims)
	usuario := usuarioDosClaims(ctx, claims, perfil)

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "auth.login", Resultado: "sucesso",
		Ator: &auditoria.Ator{Sub: usuario.ID, Perfil: string(perfil), Email: usuario.Email},
	}, req)

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
	// autodeclarar ORIENTADOR. Cadastro público sempre cria ALUNO; contas de
	// orientador só existem via AdminCreateUser (fora deste endpoint).
	novo.Perfil = common.Aluno

	if err := cognitoSignUp(ctx, novo); err != nil {
		// O erro cru vai pro CloudWatch; o cliente recebe uma mensagem
		// que diz o que corrigir (ver mensagemDeErroNoCadastro).
		log.Printf("POST /auth/registrar: %v", err)
		status, mensagem := mensagemDeErroNoCadastro(err)
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "auth.cadastro", Resultado: "falha", Motivo: mensagem,
			Ator: &auditoria.Ator{Email: novo.Email},
		}, req)
		return common.Erro(status, mensagem), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "auth.cadastro", Resultado: "sucesso",
		Ator: &auditoria.Ator{Email: novo.Email},
	}, req)

	// SignUp não retorna token — o Cognito pode exigir confirmação por
	// e-mail antes do primeiro login (ver ConfirmSignUp). O frontend
	// precisa saber disso pra não tentar redirecionar como se o usuário
	// já estivesse autenticado.
	return common.JSON(202, map[string]string{
		"mensagem": "cadastro recebido — confirme o código enviado por e-mail antes de entrar",
	}), nil
}

// HandleConfirmar implementa POST /auth/confirmar — recebe o código do
// e-mail e confirma a conta no Cognito.
func HandleConfirmar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var c ConfirmacaoCadastro
	if err := json.Unmarshal([]byte(req.Body), &c); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	c.Email = strings.TrimSpace(strings.ToLower(c.Email))
	c.Codigo = strings.TrimSpace(c.Codigo)
	if c.Email == "" || c.Codigo == "" {
		return common.Erro(400, "informe o e-mail e o código"), nil
	}

	if err := cognitoConfirmar(ctx, c); err != nil {
		log.Printf("POST /auth/confirmar: %v", err)
		status, mensagem := mensagemDeErroNaConfirmacao(err)
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "auth.conta_confirmada", Resultado: "falha", Motivo: mensagem,
			Ator: &auditoria.Ator{Email: c.Email},
		}, req)
		return common.Erro(status, mensagem), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "auth.conta_confirmada", Resultado: "sucesso",
		Ator: &auditoria.Ator{Email: c.Email},
	}, req)

	return common.JSON(200, map[string]string{
		"mensagem": "conta confirmada — você já pode entrar",
	}), nil
}

// HandleReenviarCodigo implementa POST /auth/reenviar-codigo.
func HandleReenviarCodigo(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var r ReenvioCodigo
	if err := json.Unmarshal([]byte(req.Body), &r); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	r.Email = strings.TrimSpace(strings.ToLower(r.Email))
	if r.Email == "" {
		return common.Erro(400, "informe o e-mail"), nil
	}

	if err := cognitoReenviarCodigo(ctx, r.Email); err != nil {
		log.Printf("POST /auth/reenviar-codigo: %v", err)
		status, mensagem := mensagemDeErroNaConfirmacao(err)
		return common.Erro(status, mensagem), nil
	}
	return common.JSON(200, map[string]string{
		"mensagem": "enviamos um código novo — confira o e-mail (e a caixa de spam)",
	}), nil
}

// mensagemDeErroNaConfirmacao traduz os erros de ConfirmSignUp e
// ResendConfirmationCode.
func mensagemDeErroNaConfirmacao(err error) (status int, mensagem string) {
	var (
		codigoErrado *types.CodeMismatchException
		expirado     *types.ExpiredCodeException
		jaConfirmado *types.NotAuthorizedException
		inexistente  *types.UserNotFoundException
		limite       *types.LimitExceededException
		excesso      *types.TooManyRequestsException
		tentativas   *types.TooManyFailedAttemptsException
	)
	switch {
	case errors.As(err, &codigoErrado):
		return 400, "código incorreto — confira o e-mail e tente de novo"
	case errors.As(err, &expirado):
		return 400, "código expirado — peça um código novo"
	case errors.As(err, &jaConfirmado):
		// ConfirmSignUp numa conta já CONFIRMED devolve NotAuthorized.
		return 409, "esta conta já está confirmada — é só entrar"
	case errors.As(err, &inexistente):
		return 404, "não há cadastro com esse e-mail"
	case errors.As(err, &limite), errors.As(err, &excesso), errors.As(err, &tentativas):
		return 429, "muitas tentativas seguidas — aguarde alguns minutos e tente de novo"
	default:
		return 400, "não foi possível confirmar a conta"
	}
}

// mensagemDeErroNoCadastro traduz o erro do SignUp numa mensagem que diz ao
// aluno o que corrigir. Antes todo erro virava "não foi possível concluir o
// cadastro" — senha fora da política e RGM não autorizado ficavam
// indistinguíveis.
func mensagemDeErroNoCadastro(err error) (status int, mensagem string) {
	var (
		senha     *types.InvalidPasswordException
		existente *types.UsernameExistsException
		preSignUp *types.UserLambdaValidationException
		parametro *types.InvalidParameterException
		excesso   *types.TooManyRequestsException
	)
	switch {
	case errors.As(err, &senha):
		// Espelha a política do User Pool (mín. 8, com maiúscula, minúscula,
		// número e símbolo).
		return 400, "a senha precisa ter ao menos 8 caracteres, com letra maiúscula, letra minúscula, número e símbolo"
	case errors.As(err, &existente):
		return 409, "já existe uma conta com esse e-mail — use \"Entrar\" ou recupere a senha"
	case errors.As(err, &preSignUp):
		// Recusa do Pre Sign-up Lambda: domínio errado ou RGM fora da
		// allowlist.
		return 403, "cadastro não autorizado: use o e-mail <seu RGM>@alunos.umc.br e confirme com a coordenação se o seu RGM foi pré-autorizado"
	case errors.As(err, &parametro):
		return 400, "dados inválidos — confira o nome e o e-mail"
	case errors.As(err, &excesso):
		return 429, "muitas tentativas seguidas — aguarde alguns minutos e tente de novo"
	default:
		return 400, "não foi possível concluir o cadastro"
	}
}

// perfilDosClaims lê custom:perfil do JWT recém-emitido. Contas
// self-registradas (aluno) nunca têm esse atributo setado — por isso o
// default é ALUNO quando ausente. Contas de orientador/admin são criadas
// via AdminCreateUser com o atributo explícito (fora deste endpoint, feito
// pelo admin).
func perfilDosClaims(claims map[string]any) Perfil {
	if raw, ok := claims["custom:perfil"].(string); ok && raw != "" {
		return Perfil(raw)
	}
	return common.Aluno
}
