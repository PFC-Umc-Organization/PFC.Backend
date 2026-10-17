package app

import (
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auth"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/usuario"
)

func legacyRoutes(r *httpapi.Router) {
	l := httpapi.Legacy

	// Autenticação
	r.Handle("POST", "/auth/login", l(auth.HandleLogin))
	r.Handle("POST", "/auth/registrar", l(auth.HandleRegistrar))
	r.Handle("POST", "/auth/confirmar", l(auth.HandleConfirmar))
	r.Handle("POST", "/auth/reenviar-codigo", l(auth.HandleReenviarCodigo))

	// Usuários (contas do Cognito)
	r.Handle("GET", "/usuarios", l(usuario.HandleListar))
	r.Handle("POST", "/admin/usuarios", l(usuario.HandleCriarConta))
}
