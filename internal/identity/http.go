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
	r.Handle("POST", "/auth/nova-senha", h.completeNewPassword)
	r.Handle("POST", "/auth/esqueci-senha", h.forgotPassword)
	r.Handle("POST", "/auth/redefinir-senha", h.resetPassword)
	r.Handle("GET", "/me", h.me)
	r.Handle("GET", "/usuarios", httpapi.Restrict(httpapi.Policy{
		Roles:   []actor.Role{actor.RoleAdmin, actor.RoleAdvisor},
		Message: msgStaffOnly,
	}, h.listUsers))
	r.Handle("POST", "/admin/usuarios", admin("usuario.criado", h.createAccount))
	r.Handle("PUT", "/admin/usuarios/:usuarioId/nome", admin("usuario.nome_alterado", h.renameAccount))
	r.Handle("PUT", "/admin/usuarios/:usuarioId/perfil", admin("usuario.perfil_alterado", h.changeRole))
	r.Handle("POST", "/admin/usuarios/:usuarioId/desativar", admin("usuario.desativado", h.disableAccount))
	r.Handle("POST", "/admin/usuarios/:usuarioId/reativar", admin("usuario.reativado", h.enableAccount))
	r.Handle("POST", "/admin/usuarios/:usuarioId/convite", admin("usuario.convite_reenviado", h.resendInvitation))
}

func admin(auditAction string, h httpapi.Handler) httpapi.Handler {
	return httpapi.Restrict(httpapi.Policy{
		Roles:       []actor.Role{actor.RoleAdmin},
		Message:     msgAdminOnly,
		AuditAction: auditAction,
	}, h)
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
		"mensagem": "Cadastro recebido. Para entrar, confirme o código enviado ao seu e-mail.",
	}, h.svc.SignUp(ctx, in))
}

func (h handlers) confirm(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in Confirmation
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Conta confirmada. Você já pode entrar.", h.svc.Confirm(ctx, in))
}

func (h handlers) resendCode(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ResendInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Se houver um cadastro aguardando confirmação com este e-mail, um novo código foi enviado. Verifique também a caixa de spam.", h.svc.ResendCode(ctx, in))
}

func (h handlers) completeNewPassword(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in NewPasswordInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	session, err := h.svc.CompleteNewPassword(ctx, in)
	return httpapi.Reply(200, session, err)
}

func (h handlers) forgotPassword(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ForgotPasswordInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Se houver uma conta com este e-mail, um código para redefinir a senha foi enviado.", h.svc.ForgotPassword(ctx, in))
}

func (h handlers) resetPassword(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ResetPasswordInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Senha alterada. Você já pode entrar.", h.svc.ResetPassword(ctx, in))
}

func (h handlers) me(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	u, err := h.svc.Me(ctx, r.Actor)
	return httpapi.Reply(200, u, err)
}

func (h handlers) listUsers(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	users, err := h.svc.ListUsers(ctx, r.Actor)
	return httpapi.Reply(200, users, err)
}

func (h handlers) renameAccount(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in NameInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Nome atualizado.", h.svc.RenameAccount(ctx, r.Param("usuarioId"), in))
}

func (h handlers) changeRole(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in RoleInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Perfil alterado. A mudança vale a partir da próxima entrada da pessoa no portal.",
		h.svc.ChangeRole(ctx, r.Actor, r.Param("usuarioId"), in))
}

func (h handlers) disableAccount(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("Conta desativada. A pessoa não consegue mais entrar no portal.",
		h.svc.DisableAccount(ctx, r.Actor, r.Param("usuarioId")))
}

func (h handlers) enableAccount(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("Conta reativada.", h.svc.EnableAccount(ctx, r.Param("usuarioId")))
}

func (h handlers) resendInvitation(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("Convite reenviado. A pessoa receberá por e-mail uma nova senha temporária.",
		h.svc.ResendInvitation(ctx, r.Param("usuarioId")))
}

func (h handlers) createAccount(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in NewAccount
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Reply(201, map[string]string{
		"mensagem": "Conta criada. A pessoa receberá por e-mail uma senha temporária de acesso.",
	}, h.svc.CreateAccount(ctx, in))
}
