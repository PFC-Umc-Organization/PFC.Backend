package usuario

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

func conta(username string, enabled bool, status types.UserStatusType, atributos map[string]string) types.UserType {
	u := types.UserType{Username: aws.String(username), Enabled: enabled, UserStatus: status}
	for nome, valor := range atributos {
		u.Attributes = append(u.Attributes, types.AttributeType{Name: aws.String(nome), Value: aws.String(valor)})
	}
	return u
}

func TestUsuarioDoCognito(t *testing.T) {
	casos := []struct {
		nome  string
		conta types.UserType
		want  Usuario
	}{
		{
			nome: "aluno self sign-up: sem custom:perfil, RGM vem do e-mail",
			conta: conta("sub-aluno", true, types.UserStatusTypeConfirmed, map[string]string{
				"sub": "sub-aluno", "email": "12345678901@alunos.umc.br", "name": "Aluno Teste",
			}),
			want: Usuario{
				ID: "sub-aluno", Nome: "Aluno Teste", Email: "12345678901@alunos.umc.br",
				Perfil: "ALUNO", Status: "ATIVO", Confirmado: true, CursoIds: []string{}, RGM: "12345678901",
			},
		},
		{
			nome: "coordenador com e-mail @umc.br não tem RGM",
			conta: conta("sub-coord", true, types.UserStatusTypeConfirmed, map[string]string{
				"sub": "sub-coord", "email": "coordenacao@umc.br", "name": "Coordenação Teste", "custom:perfil": "COORDENADOR",
			}),
			want: Usuario{
				ID: "sub-coord", Nome: "Coordenação Teste", Email: "coordenacao@umc.br",
				Perfil: "COORDENADOR", Status: "ATIVO", Confirmado: true, CursoIds: []string{},
			},
		},
		{
			nome: "conta desabilitada e não confirmada; id cai pro Username",
			conta: conta("sub-inativo", false, types.UserStatusTypeUnconfirmed, map[string]string{
				"email": "22222222@alunos.umc.br", "name": "Fulano",
			}),
			want: Usuario{
				ID: "sub-inativo", Nome: "Fulano", Email: "22222222@alunos.umc.br",
				Perfil: "ALUNO", Status: "INATIVO", Confirmado: false, CursoIds: []string{}, RGM: "22222222",
			},
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := usuarioDoCognito(c.conta); !reflect.DeepEqual(got, c.want) {
				t.Errorf("\n got: %+v\nwant: %+v", got, c.want)
			}
		})
	}
}

func comPerfil(perfil string) events.APIGatewayProxyRequest {
	req := events.APIGatewayProxyRequest{}
	if perfil != "" {
		req.RequestContext.Authorizer = map[string]interface{}{
			"claims": map[string]interface{}{"custom:perfil": perfil},
		}
	}
	return req
}

func TestHandleListar(t *testing.T) {
	anterior := listarContas
	t.Cleanup(func() { listarContas = anterior })
	listarContas = func(context.Context) ([]Usuario, error) {
		return []Usuario{
			{ID: "1", Nome: "Aluno", Email: "1@alunos.umc.br", Perfil: "ALUNO", RGM: "1", CursoIds: []string{}},
			{ID: "2", Nome: "Coord", Email: "coord@umc.br", Perfil: "COORDENADOR", CursoIds: []string{}},
		}, nil
	}

	casos := map[string]bool{ // perfil de quem chama → recebe e-mail?
		"COORDENADOR": true,
		"PROFESSOR":   true,
		"ALUNO":       false,
		"":            false, // aluno self sign-up não tem custom:perfil
	}
	for perfil, recebeEmail := range casos {
		resp, _ := HandleListar(context.Background(), comPerfil(perfil))
		if resp.StatusCode != 200 {
			t.Fatalf("perfil %q: status %d", perfil, resp.StatusCode)
		}
		var usuarios []Usuario
		if err := json.Unmarshal([]byte(resp.Body), &usuarios); err != nil {
			t.Fatalf("perfil %q: resposta não é JSON: %v", perfil, err)
		}
		if len(usuarios) != 2 || usuarios[0].RGM != "1" || usuarios[1].Nome != "Coord" {
			t.Errorf("perfil %q: lista inesperada %+v", perfil, usuarios)
		}
		for _, u := range usuarios {
			if (u.Email != "") != recebeEmail {
				t.Errorf("perfil %q: e-mail de %s = %q, recebeEmail=%v", perfil, u.Nome, u.Email, recebeEmail)
			}
		}
	}

	listarContas = func(context.Context) ([]Usuario, error) { return nil, errors.New("AccessDenied") }
	if resp, _ := HandleListar(context.Background(), comPerfil("COORDENADOR")); resp.StatusCode != 500 {
		t.Errorf("erro do Cognito: status = %d, want 500", resp.StatusCode)
	}
}
