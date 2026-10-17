package app

import (
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auth"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/referencia"
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

	// Referências bibliográficas (OpenAlex + Crossref)
	r.Handle("GET", "/referencias/busca", l(referencia.HandleBuscar))
	r.Handle("GET", "/referencias/doi", l(referencia.HandleConsultarDOI))
	r.Handle("GET", "/projetos/:projetoId/referencias", l(referencia.HandleListar))
	r.Handle("POST", "/projetos/:projetoId/referencias", l(referencia.HandleAdicionar))
	r.Handle("DELETE", "/projetos/:projetoId/referencias/:referenciaId", l(referencia.HandleRemover))
}
