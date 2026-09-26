package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

var (
	cognitoClientID = os.Getenv("COGNITO_CLIENT_ID")
	cip             *cognitoidentityprovider.Client
)

func init() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(fmt.Sprintf("falha ao carregar config AWS: %v", err))
	}
	cip = cognitoidentityprovider.NewFromConfig(cfg)
}

// cognitoLogin troca email/senha por um IdToken via InitiateAuth. Não
// precisamos validar a assinatura do token aqui: ele acabou de ser emitido
// pelo próprio Cognito nesta mesma chamada, é implicitamente confiável
// dentro deste processo — validar assinatura importa quando o token chega
// de fora (ex: no authorizer do API Gateway em rotas protegidas), não aqui.
func cognitoLogin(ctx context.Context, cred Credenciais) (idToken string, err error) {
	out, err := cip.InitiateAuth(ctx, &cognitoidentityprovider.InitiateAuthInput{
		AuthFlow: types.AuthFlowTypeUserPasswordAuth,
		ClientId: aws.String(cognitoClientID),
		AuthParameters: map[string]string{
			"USERNAME": cred.Email,
			"PASSWORD": cred.Senha,
		},
	})
	if err != nil {
		return "", err
	}

	if out.ChallengeName != "" {
		// Ex: NEW_PASSWORD_REQUIRED — não deveria acontecer no fluxo de
		// self sign-up (a senha já é definida pelo próprio usuário no
		// cadastro), mas pode acontecer se o usuário foi criado via
		// AdminCreateUser sem senha permanente. Repassamos como erro
		// explícito em vez de deixar o frontend receber um token vazio.
		return "", fmt.Errorf("challenge pendente: %s (contate o administrador)", out.ChallengeName)
	}

	return *out.AuthenticationResult.IdToken, nil
}

// cognitoSignUp cria o usuário no Cognito. O RGM embutido no email é
// validado pelo Pre Sign-up Lambda do lado do Cognito (contra a allowlist
// no DynamoDB) — esta função não reimplementa essa checagem, só repassa o
// erro que o Cognito devolver se a validação falhar lá.
func cognitoSignUp(ctx context.Context, novo NovoUsuario) error {
	_, err := cip.SignUp(ctx, &cognitoidentityprovider.SignUpInput{
		ClientId: aws.String(cognitoClientID),
		Username: aws.String(novo.Email),
		Password: aws.String(novo.Senha),
		UserAttributes: []types.AttributeType{
			{Name: aws.String("email"), Value: aws.String(novo.Email)},
			{Name: aws.String("name"), Value: aws.String(novo.Nome)},
		},
	})
	return err
}

// claimsDoIdToken decodifica o payload do JWT (sem validar assinatura —
// motivo idêntico ao de cognitoLogin: token recém-emitido pelo Cognito
// nesta mesma requisição).
func claimsDoIdToken(idToken string) (map[string]any, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("token mal formado")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("falha ao decodificar payload do token: %w", err)
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("falha ao parsear claims: %w", err)
	}
	return claims, nil
}

// usuarioDosClaims monta o Usuario que o frontend espera a partir do JWT.
//
// TODO: cursoIds hoje sempre vem vazio. Matrícula em curso é dado de
// negócio que ainda não tem lugar definido — quando o desenho de
// matriculação for fechado (provavelmente mais um atributo na allowlist do
// DynamoDB), popular aqui.
func usuarioDosClaims(claims map[string]any, perfil Perfil) Usuario {
	email, _ := claims["email"].(string)
	nome, _ := claims["name"].(string)
	sub, _ := claims["sub"].(string)

	u := Usuario{
		ID:       sub,
		Nome:     nome,
		Email:    email,
		Perfil:   perfil,
		Status:   StatusAtivo,
		CursoIds: []string{},
	}
	if perfil == PerfilAluno {
		u.RGM = common.RGMDoEmail(email)
	}
	return u
}
