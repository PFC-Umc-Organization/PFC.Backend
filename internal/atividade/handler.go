package atividade

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"slices"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auditoria"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/projeto"
)

const msgSoEquipe = "acesso restrito à equipe acadêmica"

// HandleListar implementa GET /atividades. Aberto a qualquer autenticado:
// o aluno precisa dos campos pra montar o formulário de entrega.
func HandleListar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	atividades, err := listar(ctx)
	if err != nil {
		log.Printf("GET /atividades: %v", err)
		return common.Erro(500, "falha ao listar atividades"), nil
	}
	return common.JSON(200, atividades), nil
}

// HandleCriar implementa POST /atividades. Sem `campos` no corpo, a
// atividade nasce com um campo de entrega padrão (arquivo obrigatório).
func HandleCriar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !common.PerfilPermitido(req, common.Admin, common.Orientador) {
		negar(ctx, req, "atividade.criada", "")
		return common.Erro(403, msgSoEquipe), nil
	}

	var body NovaAtividade
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	dados, msg := normalizarDados(body.Titulo, body.Descricao, body.Prazo)
	if msg != "" {
		return common.Erro(400, msg), nil
	}

	campos, msg := normalizarCampos(body.Campos)
	if msg != "" {
		return common.Erro(400, msg), nil
	}

	a, err := salvar(ctx, dados, campos)
	if err != nil {
		log.Printf("POST /atividades: %v", err)
		return common.Erro(500, "falha ao criar atividade"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "atividade.criada", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "atividade", ID: a.ID},
		Detalhes: map[string]any{"titulo": a.Titulo, "prazo": a.Prazo},
	}, req)
	return common.JSON(201, a), nil
}

// HandleAtualizar implementa PUT /atividades/:atividadeId.
func HandleAtualizar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	id := req.PathParameters["atividadeId"]
	if !common.PerfilPermitido(req, common.Admin, common.Orientador) {
		negar(ctx, req, "atividade.atualizada", id)
		return common.Erro(403, msgSoEquipe), nil
	}

	var body AtualizarAtividade
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	dados, msg := normalizarDados(body.Titulo, body.Descricao, body.Prazo)
	if msg != "" {
		return common.Erro(400, msg), nil
	}

	if err := atualizar(ctx, id, dados); err != nil {
		if errors.Is(err, errNaoEncontrada) {
			return common.Erro(404, "atividade não encontrada"), nil
		}
		log.Printf("PUT /atividades/%s: %v", id, err)
		return common.Erro(500, "falha ao atualizar atividade"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "atividade.atualizada", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "atividade", ID: id},
		Detalhes: map[string]any{"titulo": dados.Titulo, "prazo": dados.Prazo},
	}, req)

	a, ok, err := buscar(ctx, id)
	if err != nil || !ok {
		return common.JSON(200, map[string]string{"mensagem": "atividade atualizada"}), nil
	}
	return common.JSON(200, a), nil
}

// HandleDeletar implementa DELETE /atividades/:atividadeId (e apaga as
// entregas dela).
func HandleDeletar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	id := req.PathParameters["atividadeId"]
	if !common.PerfilPermitido(req, common.Admin, common.Orientador) {
		negar(ctx, req, "atividade.removida", id)
		return common.Erro(403, msgSoEquipe), nil
	}

	if err := deletar(ctx, id); err != nil {
		if errors.Is(err, errNaoEncontrada) {
			return common.Erro(404, "atividade não encontrada"), nil
		}
		log.Printf("DELETE /atividades/%s: %v", id, err)
		return common.Erro(500, "falha ao remover atividade"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "atividade.removida", Resultado: "sucesso",
		Recurso: &auditoria.Recurso{Tipo: "atividade", ID: id},
	}, req)
	return common.JSON(200, map[string]string{"mensagem": "atividade removida"}), nil
}

// HandleAdicionarCampo implementa POST /atividades/:atividadeId/campos.
func HandleAdicionarCampo(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	id := req.PathParameters["atividadeId"]
	if !common.PerfilPermitido(req, common.Admin, common.Orientador) {
		negar(ctx, req, "atividade.campo_adicionado", id)
		return common.Erro(403, msgSoEquipe), nil
	}

	var body NovoCampo
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	novo, msg := normalizarCampo(body)
	if msg != "" {
		return common.Erro(400, msg), nil
	}

	campo, err := adicionarCampo(ctx, id, novo)
	switch {
	case errors.Is(err, errNaoEncontrada):
		return common.Erro(404, "atividade não encontrada"), nil
	case errors.Is(err, errLimiteDeCampos):
		return common.Erro(409, "limite de campos da atividade atingido"), nil
	case err != nil:
		log.Printf("POST /atividades/%s/campos: %v", id, err)
		return common.Erro(500, "falha ao adicionar campo"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "atividade.campo_adicionado", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "atividade", ID: id},
		Detalhes: map[string]any{"campoId": campo.ID, "rotulo": campo.Rotulo, "tipo": string(campo.Tipo)},
	}, req)
	return common.JSON(201, campo), nil
}

// HandleRemoverCampo implementa DELETE /atividades/:atividadeId/campos/:campoId.
// Recusa o último campo — toda atividade precisa de ao menos um.
func HandleRemoverCampo(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	id := req.PathParameters["atividadeId"]
	campoID := req.PathParameters["campoId"]
	if !common.PerfilPermitido(req, common.Admin, common.Orientador) {
		negar(ctx, req, "atividade.campo_removido", id)
		return common.Erro(403, msgSoEquipe), nil
	}

	err := removerCampo(ctx, id, campoID)
	switch {
	case errors.Is(err, errNaoEncontrada):
		return common.Erro(404, "atividade não encontrada"), nil
	case errors.Is(err, errCampoNaoExiste):
		return common.Erro(404, "campo não encontrado"), nil
	case errors.Is(err, errUltimoCampo):
		return common.Erro(409, errUltimoCampo.Error()), nil
	case err != nil:
		log.Printf("DELETE /atividades/%s/campos/%s: %v", id, campoID, err)
		return common.Erro(500, "falha ao remover campo"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "atividade.campo_removido", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "atividade", ID: id},
		Detalhes: map[string]any{"campoId": campoID},
	}, req)
	return common.JSON(200, map[string]string{"mensagem": "campo removido"}), nil
}

// HandleListarEntregasDaAtividade implementa GET /atividades/:atividadeId/entregas
// (visão do professor: quem já entregou).
func HandleListarEntregasDaAtividade(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !common.PerfilPermitido(req, common.Admin, common.Orientador) {
		return common.Erro(403, msgSoEquipe), nil
	}

	entregas, err := entregasDaAtividade(ctx, req.PathParameters["atividadeId"])
	if err != nil {
		log.Printf("GET /atividades/:id/entregas: %v", err)
		return common.Erro(500, "falha ao listar entregas"), nil
	}
	return common.JSON(200, entregas), nil
}

// HandleListarEntregasDoProjeto implementa GET /projetos/:projetoId/entregas.
func HandleListarEntregasDoProjeto(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	p, resp, ok := carregarProjeto(ctx, req)
	if !ok {
		return resp, nil
	}
	if !podeVer(req, p) {
		return common.Erro(403, "acesso restrito aos integrantes do projeto e à equipe acadêmica"), nil
	}

	entregas, err := entregasDoProjeto(ctx, p.ID)
	if err != nil {
		log.Printf("GET /projetos/%s/entregas: %v", p.ID, err)
		return common.Erro(500, "falha ao listar entregas"), nil
	}
	return common.JSON(200, entregas), nil
}

// HandleEntregar implementa PUT /projetos/:projetoId/entregas/:atividadeId.
// Só integrante entrega; reenviar sobrescreve a entrega anterior.
func HandleEntregar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	p, resp, ok := carregarProjeto(ctx, req)
	if !ok {
		return resp, nil
	}
	atividadeID := req.PathParameters["atividadeId"]

	if !ehIntegrante(req, p) {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "entrega.enviada", Resultado: "falha",
			Motivo:  "apenas integrantes do projeto podem entregar",
			Recurso: &auditoria.Recurso{Tipo: "projeto", ID: p.ID},
		}, req)
		return common.Erro(403, "apenas integrantes do projeto podem entregar"), nil
	}

	var body NovaEntrega
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}

	a, existe, err := buscar(ctx, atividadeID)
	if err != nil {
		log.Printf("PUT /projetos/%s/entregas/%s: %v", p.ID, atividadeID, err)
		return common.Erro(500, "falha ao carregar atividade"), nil
	}
	if !existe {
		return common.Erro(404, "atividade não encontrada"), nil
	}

	respostas, msg := validarRespostas(a.Campos, body.Respostas)
	if msg != "" {
		return common.Erro(400, msg), nil
	}

	e, err := salvarEntrega(ctx, p.ID, a.ID, common.NomeDaRequisicao(req), respostas)
	if err != nil {
		log.Printf("PUT /projetos/%s/entregas/%s: %v", p.ID, atividadeID, err)
		return common.Erro(500, "falha ao salvar entrega"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "entrega.enviada", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "atividade", ID: a.ID},
		Detalhes: map[string]any{"projetoId": p.ID},
	}, req)
	return common.JSON(200, e), nil
}

// HandleRemoverEntrega implementa DELETE /projetos/:projetoId/entregas/:atividadeId.
// Só a equipe acadêmica: é como o orientador devolve a atividade pro grupo
// refazer. O conteúdo da entrega não é editado por ninguém além do grupo.
func HandleRemoverEntrega(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	projetoID := req.PathParameters["projetoId"]
	atividadeID := req.PathParameters["atividadeId"]
	if !common.PerfilPermitido(req, common.Admin, common.Orientador) {
		negar(ctx, req, "entrega.removida", atividadeID)
		return common.Erro(403, msgSoEquipe), nil
	}

	if err := removerEntrega(ctx, projetoID, atividadeID); err != nil {
		if errors.Is(err, errEntregaNaoExiste) {
			return common.Erro(404, "entrega não encontrada"), nil
		}
		log.Printf("DELETE /projetos/%s/entregas/%s: %v", projetoID, atividadeID, err)
		return common.Erro(500, "falha ao remover entrega"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "entrega.removida", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "atividade", ID: atividadeID},
		Detalhes: map[string]any{"projetoId": projetoID},
	}, req)
	return common.JSON(200, map[string]string{"mensagem": "entrega removida"}), nil
}

func negar(ctx context.Context, req events.APIGatewayProxyRequest, acao, atividadeID string) {
	ev := auditoria.Evento{Acao: acao, Resultado: "falha", Motivo: msgSoEquipe}
	if atividadeID != "" {
		ev.Recurso = &auditoria.Recurso{Tipo: "atividade", ID: atividadeID}
	}
	auditoria.Registrar(ctx, ev, req)
}

// carregarProjeto busca o projeto do path. ok=false traz a resposta de erro
// pronta pra devolver.
func carregarProjeto(ctx context.Context, req events.APIGatewayProxyRequest) (projeto.Projeto, events.APIGatewayProxyResponse, bool) {
	p, existe, err := projeto.Buscar(ctx, req.PathParameters["projetoId"])
	if err != nil {
		return projeto.Projeto{}, common.Erro(500, "falha ao carregar projeto"), false
	}
	if !existe {
		return projeto.Projeto{}, common.Erro(404, "projeto não encontrado"), false
	}
	return p, events.APIGatewayProxyResponse{}, true
}

// ehIntegrante: o integrante é identificado pelo RGM do e-mail (aluno).
// Admin/orientador nunca entregam pelo grupo.
func ehIntegrante(req events.APIGatewayProxyRequest, p projeto.Projeto) bool {
	if common.PerfilPermitido(req, common.Admin, common.Orientador) {
		return false
	}
	rgm := common.RGMDaRequisicao(req)
	return rgm != "" && slices.Contains(p.Integrantes, rgm)
}

// podeVer: equipe acadêmica acompanha qualquer PFC; o aluno só o do grupo.
func podeVer(req events.APIGatewayProxyRequest, p projeto.Projeto) bool {
	return common.PerfilPermitido(req, common.Admin, common.Orientador) || ehIntegrante(req, p)
}
