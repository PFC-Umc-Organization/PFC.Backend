package groups

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

func TestStudentGuard(t *testing.T) {
	var buf bytes.Buffer
	t.Cleanup(audit.SetOutput(&buf))
	called := false
	r := httpapi.NewRouter()
	r.Handle("POST", "/x", student(msgStudentsOnlyCreate, "projeto.criado_pelo_aluno", func(context.Context, *httpapi.Request) (httpapi.Response, error) {
		called = true
		return httpapi.JSON(201, "ok"), nil
	}))
	call := func(role, email string) int {
		resp, _ := r.Lambda(context.Background(), events.APIGatewayProxyRequest{HTTPMethod: "POST", Path: "/x",
			RequestContext: events.APIGatewayProxyRequestContext{Authorizer: map[string]any{
				"claims": map[string]any{"custom:perfil": role, "email": email},
			}}})
		return resp.StatusCode
	}

	if call("ADMIN", "a@umc.br") != 403 || call("ORIENTADOR", "o@umc.br") != 403 || call("ALUNO", "") != 403 || called {
		t.Fatal("staff and accounts without RGM must be denied")
	}
	if !strings.Contains(buf.String(), `"motivo":"rota exclusiva de aluno"`) || !strings.Contains(buf.String(), `"motivo":"conta sem RGM"`) {
		t.Errorf("audit = %s", buf.String())
	}
	if call("", "123@alunos.umc.br") != 201 || !called {
		t.Fatal("a self-registered student without role must pass")
	}
}
