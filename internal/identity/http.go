package identity

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

type handlers struct{ svc *Service }

// The /auth routes have no authorizer: there is no session yet.
func Routes(r *httpapi.Router, svc *Service) {
	h := handlers{svc}
	r.Handle("POST", "/auth/login", h.login)
	r.Handle("POST", "/auth/registrar", h.signUp)
	r.Handle("POST", "/auth/confirmar", h.confirm)
	r.Handle("POST", "/auth/reenviar-codigo", h.resendCode)
	r.Handle("GET", "/usuarios", h.listUsers)
	r.Handle("POST", "/admin/usuarios", httpapi.Restrict(httpapi.Policy{
		Roles:       []actor.Role{actor.RoleAdmin},
		Message:     msgAdminOnly,
		AuditAction: "usuario.criado",
	}, h.createAccount))
}

func (h handlers) login(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in Credentials
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	session, err := h.svc.Login(ctx, in)
	return httpapi.Reply(200, session, err)
}

// signUp answers 202: Cognito wants the e-mail code before the first login,
// so the frontend must not treat the student as signed in.
func (h handlers) signUp(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in SignUpInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Reply(202, map[string]string{
		"mensagem": "cadastro recebido — confirme o código enviado por e-mail antes de entrar",
	}, h.svc.SignUp(ctx, in))
}

func (h handlers) confirm(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in Confirmation
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("conta confirmada — você já pode entrar", h.svc.Confirm(ctx, in))
}

func (h handlers) resendCode(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ResendInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("enviamos um código novo — confira o e-mail (e a caixa de spam)", h.svc.ResendCode(ctx, in))
}

func (h handlers) listUsers(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	users, err := h.svc.ListUsers(ctx, r.Actor)
	return httpapi.Reply(200, users, err)
}

func (h handlers) createAccount(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in NewAccount
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Reply(201, map[string]string{
		"mensagem": "conta criada — um convite foi enviado por e-mail",
	}, h.svc.CreateAccount(ctx, in))
}
