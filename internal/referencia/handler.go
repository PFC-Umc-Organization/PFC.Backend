package referencia

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/projeto"
)

const (
	tamanhoMinimoBusca = 3
	tamanhoMaximoBusca = 200
)

// agora é variável pra os testes fixarem a data de "Acesso em:".
var agora = time.Now

// HandleBuscar implementa GET /referencias/busca?q=&pagina= (OpenAlex).
func HandleBuscar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	termo := strings.TrimSpace(req.QueryStringParameters["q"])
	if n := utf8.RuneCountInString(termo); n < tamanhoMinimoBusca || n > tamanhoMaximoBusca {
		return common.Erro(400, "informe de 3 a 200 caracteres para buscar"), nil
	}

	pagina := 1
	if p := req.QueryStringParameters["pagina"]; p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > paginaMaxima {
			return common.Erro(400, "página inválida"), nil
		}
		pagina = n
	}

	total, obras, err := buscarPorTema(ctx, termo, pagina)
	if err != nil {
		return common.Erro(502, "o serviço de busca de artigos está indisponível, tente novamente em instantes"), nil
	}

	artigos := make([]ArtigoEncontrado, 0, len(obras))
	for _, o := range obras {
		artigos = append(artigos, paraArtigo(o))
	}
	return common.JSON(200, ResultadoBusca{Total: total, Pagina: pagina, Artigos: artigos}), nil
}

// HandleConsultarDOI implementa GET /referencias/doi?doi= (Crossref). DOI
// vai na query e não no path porque tem "/" — o router separa por segmento.
func HandleConsultarDOI(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	doi, ok := normalizarDOI(req.QueryStringParameters["doi"])
	if !ok {
		return common.Erro(400, "DOI inválido — use o formato 10.xxxx/xxxxx"), nil
	}

	obra, err := buscarPorDOI(ctx, doi)
	if errors.Is(err, errNaoEncontrado) {
		return common.Erro(404, "DOI não encontrado"), nil
	}
	if err != nil {
		return common.Erro(502, "o serviço de consulta de DOI está indisponível, tente novamente em instantes"), nil
	}

	texto, html := formatarABNT(obra, agora())
	return common.JSON(200, ReferenciaFormatada{
		ArtigoEncontrado: paraArtigo(obra),
		ABNT:             texto,
		ABNTHTML:         html,
	}), nil
}

// HandleListar implementa GET /projetos/:projetoId/referencias.
func HandleListar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	p, resp, ok := carregarProjeto(ctx, req)
	if !ok {
		return resp, nil
	}
	if !podeVer(common.PerfilDaRequisicao(req), common.RGMDaRequisicao(req), p) {
		return common.Erro(403, "acesso restrito aos integrantes do projeto e à equipe acadêmica"), nil
	}

	referencias, err := listar(ctx, p.ID)
	if err != nil {
		return common.Erro(500, "falha ao listar referências"), nil
	}
	return common.JSON(200, referencias), nil
}

// HandleAdicionar implementa POST /projetos/:projetoId/referencias.
func HandleAdicionar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var body NovaReferencia
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	doi, ok := normalizarDOI(body.DOI)
	if !ok {
		return common.Erro(400, "DOI inválido — use o formato 10.xxxx/xxxxx"), nil
	}

	p, resp, ok := carregarProjeto(ctx, req)
	if !ok {
		return resp, nil
	}
	if !podeEditar(common.PerfilDaRequisicao(req), common.RGMDaRequisicao(req), p) {
		return common.Erro(403, "apenas integrantes do projeto podem alterar as referências"), nil
	}

	obra, err := buscarPorDOI(ctx, doi)
	if errors.Is(err, errNaoEncontrado) {
		return common.Erro(404, "DOI não encontrado"), nil
	}
	if err != nil {
		return common.Erro(502, "o serviço de consulta de DOI está indisponível, tente novamente em instantes"), nil
	}

	momento := agora()
	texto, html := formatarABNT(obra, momento)
	nova := Referencia{
		// Usa o DOI canônico do Crossref, não o que o aluno digitou.
		ID:            idDaReferencia(valorOu(obra.DOI, doi)),
		DOI:           valorOu(obra.DOI, doi),
		Titulo:        tituloCompleto(obra),
		Ano:           obra.Ano,
		ABNT:          texto,
		ABNTHTML:      html,
		AdicionadaPor: common.NomeDaRequisicao(req),
		AdicionadaEm:  momento.UTC().Format(time.RFC3339),
	}

	salva, err := salvar(ctx, p.ID, nova, common.SubDaRequisicao(req))
	if errors.Is(err, errJaExiste) {
		return common.Erro(409, "essa referência já está na lista do projeto"), nil
	}
	if err != nil {
		return common.Erro(500, "falha ao salvar referência"), nil
	}
	return common.JSON(201, salva), nil
}

// HandleRemover implementa DELETE /projetos/:projetoId/referencias/:referenciaId.
func HandleRemover(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	p, resp, ok := carregarProjeto(ctx, req)
	if !ok {
		return resp, nil
	}
	if !podeEditar(common.PerfilDaRequisicao(req), common.RGMDaRequisicao(req), p) {
		return common.Erro(403, "apenas integrantes do projeto podem alterar as referências"), nil
	}

	err := remover(ctx, p.ID, req.PathParameters["referenciaId"])
	if errors.Is(err, errNaoEncontrado) {
		return common.Erro(404, "referência não encontrada"), nil
	}
	if err != nil {
		return common.Erro(500, "falha ao remover referência"), nil
	}
	return common.JSON(200, map[string]string{"mensagem": "referência removida"}), nil
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

func ehEquipeAcademica(perfil string) bool {
	return perfil == "PROFESSOR" || perfil == "COORDENADOR"
}

// podeVer: a equipe acadêmica acompanha a bibliografia de qualquer PFC; o
// aluno só a do próprio grupo.
func podeVer(perfil, rgm string, p projeto.Projeto) bool {
	return ehEquipeAcademica(perfil) || podeEditar(perfil, rgm, p)
}

// podeEditar: a lista é do grupo, então só integrante mexe. Aluno
// auto-cadastrado não tem custom:perfil (perfil vazio = ALUNO).
func podeEditar(perfil, rgm string, p projeto.Projeto) bool {
	if ehEquipeAcademica(perfil) || rgm == "" {
		return false
	}
	return slices.Contains(p.Integrantes, rgm)
}
