package app

import (
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/atividade"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auth"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/projeto"
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

	// Projetos
	r.Handle("POST", "/meu-pfc", l(projeto.HandleCriarDoAluno))
	r.Handle("POST", "/programas/:programaId/projetos", l(projeto.HandleCriar))
	r.Handle("GET", "/programas/:programaId/projetos", l(projeto.HandleListarPorPrograma))
	r.Handle("PUT", "/projetos/:projetoId", l(projeto.HandleAtualizar))
	r.Handle("DELETE", "/projetos/:projetoId", l(projeto.HandleDeletar))
	r.Handle("PUT", "/projetos/:projetoId/orientador", l(projeto.HandleAssociarOrientador))
	r.Handle("DELETE", "/projetos/:projetoId/orientador", l(projeto.HandleRemoverOrientador))
	r.Handle("PUT", "/projetos/:projetoId/integrantes", l(projeto.HandleAdicionarIntegrante))
	r.Handle("DELETE", "/projetos/:projetoId/integrantes", l(projeto.HandleRemoverIntegrante))

	// Convites de integrante (o aluno só entra no grupo quando aceita)
	r.Handle("GET", "/meus-convites", l(projeto.HandleListarMeusConvites))
	r.Handle("POST", "/meus-convites/:projetoId/aceitar", l(projeto.HandleAceitarConvite))
	r.Handle("POST", "/meus-convites/:projetoId/recusar", l(projeto.HandleRecusarConvite))
	r.Handle("GET", "/projetos/:projetoId/convites", l(projeto.HandleListarConvitesDoGrupo))
	r.Handle("POST", "/projetos/:projetoId/convites", l(projeto.HandleConvidar))
	r.Handle("DELETE", "/projetos/:projetoId/convites/:rgm", l(projeto.HandleCancelarConvite))

	// Atividades, campos de entrega e entregas
	r.Handle("GET", "/atividades", l(atividade.HandleListar))
	r.Handle("POST", "/atividades", l(atividade.HandleCriar))
	r.Handle("PUT", "/atividades/:atividadeId", l(atividade.HandleAtualizar))
	r.Handle("DELETE", "/atividades/:atividadeId", l(atividade.HandleDeletar))
	r.Handle("POST", "/atividades/:atividadeId/campos", l(atividade.HandleAdicionarCampo))
	r.Handle("DELETE", "/atividades/:atividadeId/campos/:campoId", l(atividade.HandleRemoverCampo))
	r.Handle("GET", "/atividades/:atividadeId/entregas", l(atividade.HandleListarEntregasDaAtividade))
	r.Handle("GET", "/projetos/:projetoId/entregas", l(atividade.HandleListarEntregasDoProjeto))
	r.Handle("PUT", "/projetos/:projetoId/entregas/:atividadeId", l(atividade.HandleEntregar))
	r.Handle("DELETE", "/projetos/:projetoId/entregas/:atividadeId", l(atividade.HandleRemoverEntrega))

	// Referências bibliográficas (OpenAlex + Crossref)
	r.Handle("GET", "/referencias/busca", l(referencia.HandleBuscar))
	r.Handle("GET", "/referencias/doi", l(referencia.HandleConsultarDOI))
	r.Handle("GET", "/projetos/:projetoId/referencias", l(referencia.HandleListar))
	r.Handle("POST", "/projetos/:projetoId/referencias", l(referencia.HandleAdicionar))
	r.Handle("DELETE", "/projetos/:projetoId/referencias/:referenciaId", l(referencia.HandleRemover))
}
