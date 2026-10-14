package projeto

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"slices"
	"strings"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auditoria"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// Convite de integrante (tarefa 8). Sem SES, o convite fica salvo no
// sistema e o convidado responde pela tela: só ao aceitar o RGM entra em
// `integrantes`. Ver a modelagem em convite_dynamo.go.

// Funções variáveis pra os testes não dependerem do DynamoDB.
var (
	conviteDoAluno    = buscarConvite
	convitesRecebidos = convitesDoRGM
	convitesDoGrupo   = convitesDoProjeto
	removerConvite    = apagarConvite
	projetoPorID      = Buscar
	entrarComConvite  = entrarNoGrupo
)

// alunoDaRequisicao devolve o RGM de quem chamou, ou a resposta de erro
// pronta quando não é aluno (rotas de convite são só de aluno).
func alunoDaRequisicao(req events.APIGatewayProxyRequest) (rgm string, erro events.APIGatewayProxyResponse, ok bool) {
	if common.PerfilPermitido(req, common.Admin, common.Orientador) {
		return "", common.Erro(403, "esta rota é para alunos"), false
	}
	rgm = common.RGMDaRequisicao(req)
	if rgm == "" {
		return "", common.Erro(403, "não foi possível identificar o seu RGM"), false
	}
	return rgm, events.APIGatewayProxyResponse{}, true
}

// HandleListarMeusConvites implementa GET /meus-convites.
func HandleListarMeusConvites(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	rgm, resp, ok := alunoDaRequisicao(req)
	if !ok {
		return resp, nil
	}

	convites, err := convitesRecebidos(ctx, rgm)
	if err != nil {
		log.Printf("GET /meus-convites: %v", err)
		return common.Erro(500, "falha ao listar convites"), nil
	}
	return common.JSON(200, convites), nil
}

// HandleAceitarConvite implementa POST /meus-convites/:projetoId/aceitar.
// As regras do /meu-pfc valem de novo aqui, porque podem ter mudado desde o
// convite: o aluno precisa estar livre e na turma do grupo.
func HandleAceitarConvite(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	projetoID := req.PathParameters["projetoId"]
	falha := func(motivo string) {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "convite.aceito", Resultado: "falha", Motivo: motivo,
			Recurso: &auditoria.Recurso{Tipo: "projeto", ID: projetoID},
		}, req)
	}

	rgm, resp, ok := alunoDaRequisicao(req)
	if !ok {
		return resp, nil
	}

	_, existe, err := conviteDoAluno(ctx, projetoID, rgm)
	if err != nil {
		log.Printf("POST /meus-convites/%s/aceitar: %v", projetoID, err)
		return common.Erro(500, "falha ao carregar convite"), nil
	}
	if !existe {
		return common.Erro(404, "convite não encontrado"), nil
	}

	p, existe, err := projetoPorID(ctx, projetoID)
	if err != nil {
		log.Printf("POST /meus-convites/%s/aceitar: %v", projetoID, err)
		return common.Erro(500, "falha ao carregar o grupo"), nil
	}
	if !existe {
		// Grupo apagado depois do convite: o convite ficou órfão.
		if err := removerConvite(ctx, projetoID, rgm); err != nil {
			log.Printf("POST /meus-convites/%s/aceitar: convite órfão: %v", projetoID, err)
		}
		falha("grupo não existe mais")
		return common.Erro(404, "este grupo não existe mais"), nil
	}

	_, ocupado, err := projetoDoRGM(ctx, rgm)
	if err != nil {
		log.Printf("POST /meus-convites/%s/aceitar: %v", projetoID, err)
		return common.Erro(500, "falha ao validar a sua participação"), nil
	}
	if ocupado {
		falha("aluno já está em um projeto")
		return common.Erro(409, "você já faz parte de um projeto"), nil
	}

	turmaID, err := turmaDoRGM(ctx, rgm)
	if err != nil {
		log.Printf("POST /meus-convites/%s/aceitar: %v", projetoID, err)
		return common.Erro(500, "falha ao validar a sua matrícula"), nil
	}
	programaID, temPrograma, err := programaDaTurma(ctx, turmaID)
	if err != nil {
		log.Printf("POST /meus-convites/%s/aceitar: %v", projetoID, err)
		return common.Erro(500, "falha ao validar a sua turma"), nil
	}
	if turmaID == "" || !temPrograma || programaID != p.ProgramaID {
		falha("aluno fora da turma do grupo")
		return common.Erro(409, "você não está pré-autorizado na turma deste grupo — fale com a coordenação"), nil
	}

	if err := entrarComConvite(ctx, projetoID, rgm); err != nil {
		if errors.Is(err, errNaoPodeEntrar) {
			falha("grupo cheio ou aluno já é integrante")
			return common.Erro(409, "não foi possível entrar: o grupo está cheio ou você já é integrante"), nil
		}
		log.Printf("POST /meus-convites/%s/aceitar: %v", projetoID, err)
		return common.Erro(500, "falha ao entrar no grupo"), nil
	}

	// Cada aluno participa de um único projeto: os outros convites dele
	// perdem o sentido. Falha aqui não desfaz a entrada — só fica no log.
	pendentes, err := convitesRecebidos(ctx, rgm)
	if err != nil {
		log.Printf("POST /meus-convites/%s/aceitar: limpar convites: %v", projetoID, err)
	}
	for _, c := range pendentes {
		if err := removerConvite(ctx, c.ProjetoID, rgm); err != nil && !errors.Is(err, errConviteNaoExiste) {
			log.Printf("POST /meus-convites/%s/aceitar: limpar convite de %s: %v", projetoID, c.ProjetoID, err)
		}
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "convite.aceito", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "projeto", ID: projetoID},
		Detalhes: map[string]any{"rgm": rgm},
	}, req)

	p.Integrantes = append(p.Integrantes, rgm)
	return common.JSON(200, p), nil
}

// HandleRecusarConvite implementa POST /meus-convites/:projetoId/recusar.
func HandleRecusarConvite(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	projetoID := req.PathParameters["projetoId"]

	rgm, resp, ok := alunoDaRequisicao(req)
	if !ok {
		return resp, nil
	}

	if err := removerConvite(ctx, projetoID, rgm); err != nil {
		if errors.Is(err, errConviteNaoExiste) {
			return common.Erro(404, "convite não encontrado"), nil
		}
		log.Printf("POST /meus-convites/%s/recusar: %v", projetoID, err)
		return common.Erro(500, "falha ao recusar convite"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "convite.recusado", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "projeto", ID: projetoID},
		Detalhes: map[string]any{"rgm": rgm},
	}, req)

	return common.JSON(200, map[string]string{"mensagem": "convite recusado"}), nil
}

// carregarGrupo busca o projeto do path. ok=false traz a resposta de erro
// pronta pra devolver.
func carregarGrupo(ctx context.Context, req events.APIGatewayProxyRequest) (Projeto, events.APIGatewayProxyResponse, bool) {
	p, existe, err := projetoPorID(ctx, req.PathParameters["projetoId"])
	if err != nil {
		return Projeto{}, common.Erro(500, "falha ao carregar projeto"), false
	}
	if !existe {
		return Projeto{}, common.Erro(404, "projeto não encontrado"), false
	}
	return p, events.APIGatewayProxyResponse{}, true
}

// ehIntegranteDoGrupo: o integrante é identificado pelo RGM do e-mail
// (mesma regra de atividade.ehIntegrante). Admin/orientador nunca contam.
func ehIntegranteDoGrupo(req events.APIGatewayProxyRequest, p Projeto) bool {
	if common.PerfilPermitido(req, common.Admin, common.Orientador) {
		return false
	}
	rgm := common.RGMDaRequisicao(req)
	return rgm != "" && slices.Contains(p.Integrantes, rgm)
}

// HandleListarConvitesDoGrupo implementa GET /projetos/:projetoId/convites.
// O grupo acompanha quem ainda não respondeu; a equipe acadêmica também vê.
func HandleListarConvitesDoGrupo(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	p, resp, ok := carregarGrupo(ctx, req)
	if !ok {
		return resp, nil
	}
	if !ehIntegranteDoGrupo(req, p) && !common.PerfilPermitido(req, common.Admin, common.Orientador) {
		return common.Erro(403, "acesso restrito aos integrantes do projeto e à equipe acadêmica"), nil
	}

	convites, err := convitesDoGrupo(ctx, p.ID)
	if err != nil {
		log.Printf("GET /projetos/%s/convites: %v", p.ID, err)
		return common.Erro(500, "falha ao listar convites"), nil
	}
	return common.JSON(200, convites), nil
}

// HandleConvidar implementa POST /projetos/:projetoId/convites — um
// integrante convida mais alguém depois de o grupo existir. Mesmas regras
// do /meu-pfc; convite pendente já ocupa vaga no limite do grupo.
func HandleConvidar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	p, resp, ok := carregarGrupo(ctx, req)
	if !ok {
		return resp, nil
	}
	falha := func(motivo string) {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "convite.enviado", Resultado: "falha", Motivo: motivo,
			Recurso: &auditoria.Recurso{Tipo: "projeto", ID: p.ID},
		}, req)
	}

	if !ehIntegranteDoGrupo(req, p) {
		falha("apenas integrantes podem convidar")
		return common.Erro(403, "apenas integrantes do projeto podem convidar"), nil
	}

	var body AssociarAluno
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	colega := strings.TrimSpace(body.RGM)
	if colega == "" {
		return common.Erro(400, "rgm é obrigatório"), nil
	}
	if slices.Contains(p.Integrantes, colega) {
		return common.Erro(409, "o RGM "+colega+" já é integrante do grupo"), nil
	}

	turmaID, err := turmaDoRGM(ctx, colega)
	if err != nil {
		log.Printf("POST /projetos/%s/convites: %v", p.ID, err)
		return common.Erro(500, "falha ao validar o convidado"), nil
	}
	programaID, temPrograma, err := programaDaTurma(ctx, turmaID)
	if err != nil {
		log.Printf("POST /projetos/%s/convites: %v", p.ID, err)
		return common.Erro(500, "falha ao validar o convidado"), nil
	}
	if turmaID == "" || !temPrograma || programaID != p.ProgramaID {
		falha("convidado fora da turma do grupo")
		return common.Erro(400, "o RGM "+colega+" não está pré-autorizado na turma do grupo"), nil
	}

	_, ocupado, err := projetoDoRGM(ctx, colega)
	if err != nil {
		log.Printf("POST /projetos/%s/convites: %v", p.ID, err)
		return common.Erro(500, "falha ao validar o convidado"), nil
	}
	if ocupado {
		falha("convidado já está em um projeto")
		return common.Erro(409, "o RGM "+colega+" já faz parte de outro projeto"), nil
	}

	pendentes, err := convitesDoGrupo(ctx, p.ID)
	if err != nil {
		log.Printf("POST /projetos/%s/convites: %v", p.ID, err)
		return common.Erro(500, "falha ao validar o tamanho do grupo"), nil
	}
	if len(p.Integrantes)+len(pendentes) >= maxIntegrantes {
		falha("grupo cheio")
		return common.Erro(409, "o grupo já está cheio (contando os convites pendentes)"), nil
	}

	c, err := criarConvite(ctx, p.ID, p.Nome, colega, common.NomeDaRequisicao(req))
	if errors.Is(err, errConviteJaExiste) {
		return common.Erro(409, "o RGM "+colega+" já tem um convite deste grupo"), nil
	}
	if err != nil {
		log.Printf("POST /projetos/%s/convites: %v", p.ID, err)
		return common.Erro(500, "falha ao enviar convite"), nil
	}
	notificarConvite(c)

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "convite.enviado", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "projeto", ID: p.ID},
		Detalhes: map[string]any{"rgm": colega},
	}, req)

	return common.JSON(201, c), nil
}

// HandleCancelarConvite implementa DELETE /projetos/:projetoId/convites/:rgm.
func HandleCancelarConvite(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	p, resp, ok := carregarGrupo(ctx, req)
	if !ok {
		return resp, nil
	}
	colega := req.PathParameters["rgm"]

	if !ehIntegranteDoGrupo(req, p) {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "convite.cancelado", Resultado: "falha", Motivo: "apenas integrantes podem cancelar",
			Recurso: &auditoria.Recurso{Tipo: "projeto", ID: p.ID},
		}, req)
		return common.Erro(403, "apenas integrantes do projeto podem cancelar convites"), nil
	}

	if err := removerConvite(ctx, p.ID, colega); err != nil {
		if errors.Is(err, errConviteNaoExiste) {
			return common.Erro(404, "convite não encontrado"), nil
		}
		log.Printf("DELETE /projetos/%s/convites/%s: %v", p.ID, colega, err)
		return common.Erro(500, "falha ao cancelar convite"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "convite.cancelado", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "projeto", ID: p.ID},
		Detalhes: map[string]any{"rgm": colega},
	}, req)

	return common.JSON(200, map[string]string{"mensagem": "convite cancelado"}), nil
}
