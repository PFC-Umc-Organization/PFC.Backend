package projeto

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auditoria"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

const msgJaTemProjeto = "você já faz parte de um projeto"

const (
	tamanhoMaxNome      = 150
	tamanhoMaxDescricao = 2000
	maxIntegrantes      = 10
)

// Funções variáveis pra os testes não dependerem do DynamoDB.
var (
	turmaDoRGM      func(context.Context, string) (string, error)
	turmasDosRGMs   func(context.Context, []string) (map[string]string, error)
	programaDaTurma func(context.Context, string) (string, bool, error)
	programaExiste  func(context.Context, string) (bool, error)
	matriculado     func(context.Context, string) (bool, error)
	projetoDoRGM    = buscarPorRGM
	ocupadosDoGrupo = projetosDosRGMs
	salvarProjeto   = salvar
	criarConvite    = salvarConvite

	// notificarConvite é o gancho de e-mail do convite. Sem SES, por
	// enquanto só registra no log; com um provedor de e-mail, troca-se só
	// esta função. O convidado responde pela tela de qualquer forma.
	notificarConvite = func(c Convite) {
		log.Printf("convite: RGM %s convidado para o projeto %q (%s) por %s",
			c.RGM, c.ProjetoNome, c.ProjetoID, c.ConvidadoPor)
	}
)

// integrantesDoGrupo junta o criador e os colegas informados, sem
// repetição e sem vazios. O criador sempre vem primeiro.
func integrantesDoGrupo(criador string, colegas []string) []string {
	vistos := map[string]bool{criador: true}
	grupo := []string{criador}
	for _, rgm := range colegas {
		rgm = strings.TrimSpace(rgm)
		if rgm == "" || vistos[rgm] {
			continue
		}
		vistos[rgm] = true
		grupo = append(grupo, rgm)
	}
	return grupo
}

// HandleCriarDoAluno implementa POST /meu-pfc — o aluno pré-autorizado cria
// o projeto do grupo no programa da própria turma.
//
// Regras: só ALUNO; o RGM precisa estar vinculado a uma turma que já tenha
// programa de PFC; cada aluno participa de um único projeto; os colegas
// informados precisam estar pré-autorizados na mesma turma e livres.
func HandleCriarDoAluno(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	falha := func(motivo string) {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "projeto.criado_pelo_aluno", Resultado: "falha", Motivo: motivo,
		}, req)
	}

	if common.PerfilPermitido(req, common.Admin, common.Orientador) {
		falha("rota exclusiva de aluno")
		return common.Erro(403, "esta rota é para alunos — a coordenação cria projetos pela gestão de PFC"), nil
	}
	rgm := common.RGMDaRequisicao(req)
	if rgm == "" {
		falha("conta sem RGM")
		return common.Erro(403, "não foi possível identificar o seu RGM"), nil
	}

	var body NovoProjeto
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	nome := strings.TrimSpace(body.Nome)
	descricao := strings.TrimSpace(body.Descricao)
	switch {
	case nome == "":
		return common.Erro(400, "nome é obrigatório"), nil
	case utf8.RuneCountInString(nome) > tamanhoMaxNome:
		return common.Erro(400, "nome longo demais"), nil
	case utf8.RuneCountInString(descricao) > tamanhoMaxDescricao:
		return common.Erro(400, "descrição longa demais"), nil
	}
	grupo := integrantesDoGrupo(rgm, body.Integrantes)
	if len(grupo) > maxIntegrantes {
		return common.Erro(400, "grupo grande demais"), nil
	}

	// Turma e projeto de todo o grupo em duas leituras em lote, em vez de
	// duas leituras por integrante.
	turmas, err := turmasDosRGMs(ctx, grupo)
	if err != nil {
		log.Printf("POST /meu-pfc: turmas do grupo: %v", err)
		return common.Erro(500, "falha ao validar a sua matrícula"), nil
	}
	turmaID := turmas[rgm]
	if turmaID == "" {
		falha("RGM sem turma vinculada")
		return common.Erro(403, "o seu RGM não está pré-autorizado em nenhuma turma — fale com a coordenação"), nil
	}

	programaID, existe, err := programaDaTurma(ctx, turmaID)
	if err != nil {
		log.Printf("POST /meu-pfc: programa da turma: %v", err)
		return common.Erro(500, "falha ao validar a turma"), nil
	}
	if !existe {
		falha("turma sem programa de PFC")
		return common.Erro(409, "a sua turma ainda não tem PFC iniciado — fale com a coordenação"), nil
	}

	ocupados, err := ocupadosDoGrupo(ctx, grupo)
	if err != nil {
		log.Printf("POST /meu-pfc: projetos do grupo: %v", err)
		return common.Erro(500, "falha ao validar os integrantes"), nil
	}

	// Cada integrante (criador incluso) precisa estar na mesma turma e livre.
	for _, r := range grupo {
		if turmas[r] != turmaID {
			falha("integrante fora da turma")
			return common.Erro(400, "o RGM "+r+" não está pré-autorizado na sua turma"), nil
		}
		if _, ocupado := ocupados[r]; ocupado {
			falha("integrante já está em um projeto")
			if r == rgm {
				return common.Erro(409, msgJaTemProjeto), nil
			}
			return common.Erro(409, "o RGM "+r+" já faz parte de outro projeto"), nil
		}
	}

	// Só o criador entra na hora; os colegas recebem convite e só viram
	// integrantes quando aceitarem.
	p, err := salvarProjeto(ctx, programaID, NovoProjeto{Nome: nome, Descricao: descricao, Integrantes: []string{rgm}})
	if errors.Is(err, errAlunoJaTemProjeto) {
		// Outro pedido do mesmo aluno passou na frente.
		falha("integrante já está em um projeto")
		return common.Erro(409, msgJaTemProjeto), nil
	}
	if err != nil {
		log.Printf("POST /meu-pfc: %v", err)
		return common.Erro(500, "falha ao criar projeto"), nil
	}

	// Falha num convite não desfaz o projeto (que já existe): fica no log e
	// o grupo pode convidar de novo por POST /projetos/:projetoId/convites.
	convidados := []string{}
	for _, colega := range grupo[1:] {
		c, err := criarConvite(ctx, p.ID, p.Nome, colega, common.NomeDaRequisicao(req))
		if err != nil {
			log.Printf("POST /meu-pfc: convite pro RGM %s: %v", colega, err)
			continue
		}
		notificarConvite(c)
		convidados = append(convidados, colega)
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "projeto.criado_pelo_aluno", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "projeto", ID: p.ID},
		Detalhes: map[string]any{"programaId": programaID, "nome": p.Nome, "convidados": convidados},
	}, req)

	return common.JSON(201, p), nil
}
