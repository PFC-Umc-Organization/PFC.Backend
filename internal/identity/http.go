package identity

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

// ---------------------------------------------------------------------------
// Rotas
// ---------------------------------------------------------------------------

// Rotas
type handlers struct{ svc *Service }

// Rotas
func Routes(r *httpapi.Router, svc *Service) {
	h := handlers{svc}
	r.Public("POST", "/auth/login", h.login)
	r.Public("POST", "/auth/registrar", h.signUp)
	r.Public("POST", "/auth/confirmar", h.confirm)
	r.Public("POST", "/auth/reenviar-codigo", h.resendCode)
	r.Public("POST", "/auth/nova-senha", h.completeNewPassword)
	r.Public("POST", "/auth/esqueci-senha", h.forgotPassword)
	r.Public("POST", "/auth/redefinir-senha", h.resetPassword)
	r.Handle("GET", "/me", h.me)
	r.Handle("GET", "/usuarios", httpapi.Staff(msgStaffOnly, h.listUsers))
	r.Handle("POST", "/admin/usuarios", httpapi.Admin(msgAdminOnly, h.createAccount, httpapi.Audit("usuario.criado")))
	r.Handle("PUT", "/admin/usuarios/:usuarioId/nome", httpapi.Admin(msgAdminOnly, h.renameAccount, httpapi.Audit("usuario.nome_alterado")))
	r.Handle("PUT", "/admin/usuarios/:usuarioId/perfil", httpapi.Admin(msgAdminOnly, h.changeRole, httpapi.Audit("usuario.perfil_alterado")))
	r.Handle("PUT", "/usuarios/:usuarioId/mfa", httpapi.Admin(msgAdminOnly, h.setMFA, httpapi.Audit("usuario.mfa_alterado")))
	r.Handle("POST", "/admin/usuarios/:usuarioId/desativar", httpapi.Admin(msgAdminOnly, h.disableAccount, httpapi.Audit("usuario.desativado")))
	r.Handle("POST", "/admin/usuarios/:usuarioId/reativar", httpapi.Admin(msgAdminOnly, h.enableAccount, httpapi.Audit("usuario.reativado")))
	r.Handle("POST", "/admin/usuarios/:usuarioId/convite", httpapi.Admin(msgAdminOnly, h.resendInvitation, httpapi.Audit("usuario.convite_reenviado")))
}

// ---------------------------------------------------------------------------
// Autenticação
// ---------------------------------------------------------------------------

// Login
func (h handlers) login(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in Credentials
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	session, err := h.svc.Login(ctx, in)
	return httpapi.Reply(200, session, err)
}

// ---------------------------------------------------------------------------
// Cadastro
// ---------------------------------------------------------------------------

// Cadastro
func (h handlers) signUp(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in SignUpInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Reply(202, map[string]string{
		"mensagem": "Cadastro recebido. Para entrar, confirme o código enviado ao seu e-mail.",
	}, h.svc.SignUp(ctx, in))
}

// Confirmação
func (h handlers) confirm(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in Confirmation
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Conta confirmada. Você já pode entrar.", h.svc.Confirm(ctx, in))
}

// Reenvio
func (h handlers) resendCode(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ResendInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Se houver um cadastro aguardando confirmação com este e-mail, um novo código foi enviado. Verifique também a caixa de spam.", h.svc.ResendCode(ctx, in))
}

// ---------------------------------------------------------------------------
// Senhas
// ---------------------------------------------------------------------------

// Nova senha
func (h handlers) completeNewPassword(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in NewPasswordInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	session, err := h.svc.CompleteNewPassword(ctx, in)
	return httpapi.Reply(200, session, err)
}

// Recuperação
func (h handlers) forgotPassword(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ForgotPasswordInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Se houver uma conta com este e-mail, um código para redefinir a senha foi enviado.", h.svc.ForgotPassword(ctx, in))
}

// Redefinição
func (h handlers) resetPassword(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in ResetPasswordInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Senha alterada. Você já pode entrar.", h.svc.ResetPassword(ctx, in))
}

// ---------------------------------------------------------------------------
// Sessão
// ---------------------------------------------------------------------------

// Perfil
func (h handlers) me(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	u, err := h.svc.Me(ctx, r.Actor)
	return httpapi.Reply(200, u, err)
}

// ---------------------------------------------------------------------------
// Contas
// ---------------------------------------------------------------------------

// Listagem
func (h handlers) listUsers(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	users, err := h.svc.ListUsers(ctx, r.Actor, r.Query("incluirMfa") == "true")
	return httpapi.Reply(200, users, err)
}

// Renomeação
func (h handlers) renameAccount(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in NameInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Nome atualizado.", h.svc.RenameAccount(ctx, r.Param("usuarioId"), in))
}

// Troca
func (h handlers) changeRole(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in RoleInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Message("Perfil alterado. A mudança vale a partir da próxima entrada da pessoa no portal.",
		h.svc.ChangeRole(ctx, r.Actor, r.Param("usuarioId"), in))
}

// Alternância
func (h handlers) setMFA(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in MFAInput
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	enabled, err := h.svc.SetMFA(ctx, r.Param("usuarioId"), in)
	return httpapi.Reply(200, map[string]bool{"mfaAtivo": enabled}, err)
}

// Desativação
func (h handlers) disableAccount(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("Conta desativada. A pessoa não consegue mais entrar no portal.",
		h.svc.DisableAccount(ctx, r.Actor, r.Param("usuarioId")))
}

// Reativação
func (h handlers) enableAccount(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("Conta reativada.", h.svc.EnableAccount(ctx, r.Param("usuarioId")))
}

// Reenvio
func (h handlers) resendInvitation(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	return httpapi.Message("Convite reenviado. A pessoa receberá por e-mail uma nova senha temporária.",
		h.svc.ResendInvitation(ctx, r.Param("usuarioId")))
}

// Criação
func (h handlers) createAccount(ctx context.Context, r *httpapi.Request) (httpapi.Response, error) {
	var in NewAccount
	if err := r.Decode(&in); err != nil {
		return httpapi.Response{}, err
	}
	return httpapi.Reply(201, map[string]string{
		"mensagem": "Conta criada. A pessoa receberá por e-mail uma senha temporária de acesso.",
	}, h.svc.CreateAccount(ctx, in))
}
