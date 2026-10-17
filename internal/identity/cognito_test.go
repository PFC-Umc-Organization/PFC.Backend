package identity

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

func TestClassifySignUp(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"weak password", &types.InvalidPasswordException{Message: aws.String("Password did not conform with policy")}, errWeakPassword},
		{"e-mail taken", &types.UsernameExistsException{}, errEmailTaken},
		{"pre sign-up rejection", &types.UserLambdaValidationException{Message: aws.String("PreSignUp failed with error RGM não autorizado.")}, errSignUpRejected},
		{"invalid parameter", &types.InvalidParameterException{}, errInvalidInput},
		{"throttled", &types.TooManyRequestsException{}, errThrottled},
		// The SDK wraps the API error; errors.As must still find it.
		{"wrapped", fmt.Errorf("operation error SignUp: %w", &types.UsernameExistsException{}), errEmailTaken},
	}
	for _, c := range cases {
		if got := classifySignUp(c.err); !errors.Is(got, c.want) {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
	if classifySignUp(nil) != nil {
		t.Error("nil must stay nil")
	}
}

func TestClassifyConfirmation(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"wrong code", &types.CodeMismatchException{}, errWrongCode},
		{"expired", &types.ExpiredCodeException{}, errExpiredCode},
		{"already confirmed", &types.NotAuthorizedException{}, errAlreadyConfirmed},
		{"no account", &types.UserNotFoundException{}, errNoAccount},
		{"limit", &types.LimitExceededException{}, errThrottled},
		{"failed attempts", &types.TooManyFailedAttemptsException{}, errThrottled},
		{"wrapped", fmt.Errorf("operation error ConfirmSignUp: %w", &types.CodeMismatchException{}), errWrongCode},
	}
	for _, c := range cases {
		if got := classifyConfirmation(c.err); !errors.Is(got, c.want) {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestSignUpAttributesAlwaysStudent(t *testing.T) {
	values := map[string]string{}
	for _, a := range signUpAttributes("Fulano de Tal", "123@alunos.umc.br") {
		values[aws.ToString(a.Name)] = aws.ToString(a.Value)
	}
	if values["custom:perfil"] != "ALUNO" || values["email"] != "123@alunos.umc.br" || values["name"] != "Fulano de Tal" {
		t.Errorf("attributes = %v", values)
	}
}
