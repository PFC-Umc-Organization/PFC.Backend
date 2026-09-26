package usuario

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// O Cognito é a fonte de verdade das contas — não há cópia no DynamoDB.
// ListUsers é uma chamada administrativa (autorizada por IAM, não pelo
// client público), por isso precisa do id do User Pool e da permissão
// cognito-idp:ListUsers na role da Lambda.

var (
	userPoolID = os.Getenv("COGNITO_USER_POOL_ID")
	cip        *cognitoidentityprovider.Client
)

// listarContas é variável pra os testes do handler não dependerem do
// Cognito.
var listarContas = listarDoCognito

func init() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(fmt.Sprintf("falha ao carregar config AWS: %v", err))
	}
	cip = cognitoidentityprovider.NewFromConfig(cfg)
}

// O ListUsers devolve no máximo 60 contas por página.
const porPagina = 60

func listarDoCognito(ctx context.Context) ([]Usuario, error) {
	if userPoolID == "" {
		return nil, fmt.Errorf("COGNITO_USER_POOL_ID não configurado")
	}

	usuarios := []Usuario{}
	var token *string
	for {
		// Sem AttributesToGet de propósito: o Cognito recusa "name" nesse
		// filtro (InvalidParameterException), mesmo com o atributo no
		// schema. Sem o filtro vêm todos os atributos — os que não usamos
		// são ignorados em usuarioDoCognito.
		out, err := cip.ListUsers(ctx, &cognitoidentityprovider.ListUsersInput{
			UserPoolId:      aws.String(userPoolID),
			Limit:           aws.Int32(porPagina),
			PaginationToken: token,
		})
		if err != nil {
			return nil, err
		}

		for _, u := range out.Users {
			usuarios = append(usuarios, usuarioDoCognito(u))
		}

		if out.PaginationToken == nil || *out.PaginationToken == "" {
			break
		}
		token = out.PaginationToken
	}

	sort.Slice(usuarios, func(a, b int) bool {
		return strings.ToLower(usuarios[a].Nome) < strings.ToLower(usuarios[b].Nome)
	})
	return usuarios, nil
}

// usuarioDoCognito converte a conta do Cognito no formato que o frontend
// espera. Conta sem custom:perfil é ALUNO — é assim que o self sign-up cria
// (mesma regra do login, ver auth.perfilDosClaims).
func usuarioDoCognito(u types.UserType) Usuario {
	atributos := map[string]string{}
	for _, a := range u.Attributes {
		atributos[aws.ToString(a.Name)] = aws.ToString(a.Value)
	}

	perfil := atributos["custom:perfil"]
	if perfil == "" {
		perfil = PerfilAluno
	}

	id := atributos["sub"]
	if id == "" {
		// Com e-mail como username, o Username do Cognito já é o sub.
		id = aws.ToString(u.Username)
	}

	status := StatusAtivo
	if !u.Enabled {
		status = StatusInativo
	}

	usuario := Usuario{
		ID:         id,
		Nome:       atributos["name"],
		Email:      atributos["email"],
		Perfil:     perfil,
		Status:     status,
		Confirmado: u.UserStatus == types.UserStatusTypeConfirmed,
		CursoIds:   []string{},
	}
	if perfil == PerfilAluno {
		usuario.RGM = common.RGMDoEmail(usuario.Email)
	}
	return usuario
}
