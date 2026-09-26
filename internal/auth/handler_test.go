package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

func TestMensagemDeErroNoCadastro(t *testing.T) {
	casos := []struct {
		nome   string
		err    error
		status int
	}{
		{"senha fora da política", &types.InvalidPasswordException{Message: aws.String("Password did not conform with policy")}, 400},
		{"e-mail já cadastrado", &types.UsernameExistsException{}, 409},
		{"recusa do Pre Sign-up", &types.UserLambdaValidationException{Message: aws.String("PreSignUp failed with error RGM não autorizado.")}, 403},
		{"parâmetro inválido", &types.InvalidParameterException{}, 400},
		{"excesso de tentativas", &types.TooManyRequestsException{}, 429},
		// o SDK embrulha o erro da API — errors.As tem que achar mesmo assim
		{"erro embrulhado", fmt.Errorf("operation error SignUp: %w", &types.UsernameExistsException{}), 409},
		{"desconhecido", errors.New("timeout"), 400},
	}

	mensagens := map[string]bool{}
	for _, c := range casos {
		status, msg := mensagemDeErroNoCadastro(c.err)
		if status != c.status || msg == "" {
			t.Errorf("%s: status=%d msg=%q, want status %d", c.nome, status, msg, c.status)
		}
		mensagens[msg] = true
	}
	// cada tipo de erro conhecido tem mensagem própria (o "embrulhado" repete o de e-mail)
	if len(mensagens) != 6 {
		t.Errorf("esperava 6 mensagens distintas, veio %d", len(mensagens))
	}
}

func TestAtributosDoCadastroSempreAluno(t *testing.T) {
	// mesmo que o cliente mande outro perfil, o cadastro público grava ALUNO
	attrs := atributosDoCadastro(NovoUsuario{Nome: "Fulano de Tal", Email: "123@alunos.umc.br", Perfil: "PROFESSOR"})
	valores := map[string]string{}
	for _, a := range attrs {
		valores[aws.ToString(a.Name)] = aws.ToString(a.Value)
	}
	if valores["custom:perfil"] != "ALUNO" {
		t.Errorf("custom:perfil = %q, want ALUNO", valores["custom:perfil"])
	}
	if valores["email"] != "123@alunos.umc.br" || valores["name"] != "Fulano de Tal" {
		t.Errorf("atributos inesperados: %v", valores)
	}
}

func TestMensagemDeErroNaConfirmacao(t *testing.T) {
	casos := []struct {
		nome   string
		err    error
		status int
	}{
		{"código errado", &types.CodeMismatchException{}, 400},
		{"código expirado", &types.ExpiredCodeException{}, 400},
		{"já confirmada", &types.NotAuthorizedException{}, 409},
		{"sem cadastro", &types.UserNotFoundException{}, 404},
		{"limite", &types.LimitExceededException{}, 429},
		{"embrulhado", fmt.Errorf("operation error ConfirmSignUp: %w", &types.CodeMismatchException{}), 400},
		{"desconhecido", errors.New("timeout"), 400},
	}
	for _, c := range casos {
		if status, msg := mensagemDeErroNaConfirmacao(c.err); status != c.status || msg == "" {
			t.Errorf("%s: status=%d msg=%q, want %d", c.nome, status, msg, c.status)
		}
	}
}

func TestHandleConfirmarValidaCorpo(t *testing.T) {
	// validação local — não chega a chamar o Cognito
	for _, corpo := range []string{`nao-json`, `{"email":"123@alunos.umc.br"}`, `{"codigo":"123456"}`} {
		resp, _ := HandleConfirmar(context.Background(), events.APIGatewayProxyRequest{Body: corpo})
		if resp.StatusCode != 400 {
			t.Errorf("corpo %s: status %d, want 400", corpo, resp.StatusCode)
		}
	}
}
