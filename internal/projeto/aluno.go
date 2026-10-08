package projeto

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auditoria"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/matricula"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/programa"
)

const (
	tamanhoMaxNome      = 150
	tamanhoMaxDescricao = 2000
	maxIntegrantes      = 10
)

// Funções variáveis pra os testes não dependerem do DynamoDB.
var (
	turmaDoRGM      = matricula.TurmaDoRGM
	programaDaTurma = programa.IDDoCurso
	projetoDoRGM    = buscarPorRGM
	salvarProjeto   = salvar
)

// buscarPorRGM acha o projeto em que o RGM é integrante (Scan paginado, no
// mesmo padrão dos demais domínios de baixo volume). ok=false se não houver.
func buscarPorRGM(ctx context.Context, rgm string) (p Projeto, ok bool, err error) {
	var inicio map[string]types.AttributeValue
	for {
		out, err := ddb.Scan(ctx, &dynamodb.ScanInput{
			TableName:        aws.String(tableName),
			FilterExpression: aws.String("begins_with(PK, :prefixo) AND SK = :perfil AND contains(integrantes, :rgm)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":prefixo": &types.AttributeValueMemberS{Value: "PROJECT#"},
				":perfil":  &types.AttributeValueMemberS{Value: "PROFILE"},
				":rgm":     &types.AttributeValueMemberS{Value: rgm},
			},
			ExclusiveStartKey: inicio,
		})
		if err != nil {
			return Projeto{}, false, err
		}
		for _, i := range out.Items {
			var it item
			if err := attributevalue.UnmarshalMap(i, &it); err != nil {
				continue
			}
			return toProjeto(it), true, nil
		}
		if out.LastEvaluatedKey == nil {
			return Projeto{}, false, nil
		}
		inicio = out.LastEvaluatedKey
	}
}

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

	turmaID, err := turmaDoRGM(ctx, rgm)
	if err != nil {
		log.Printf("POST /meu-pfc: turma do RGM: %v", err)
		return common.Erro(500, "falha ao validar a sua matrícula"), nil
	}
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

	// Cada integrante (criador incluso) precisa estar na mesma turma e livre.
	for _, r := range grupo {
		if r != rgm {
			t, err := turmaDoRGM(ctx, r)
			if err != nil {
				log.Printf("POST /meu-pfc: turma do RGM %s: %v", r, err)
				return common.Erro(500, "falha ao validar os integrantes"), nil
			}
			if t != turmaID {
				falha("integrante fora da turma")
				return common.Erro(400, "o RGM "+r+" não está pré-autorizado na sua turma"), nil
			}
		}
		_, ocupado, err := projetoDoRGM(ctx, r)
		if err != nil {
			log.Printf("POST /meu-pfc: projeto do RGM %s: %v", r, err)
			return common.Erro(500, "falha ao validar os integrantes"), nil
		}
		if ocupado {
			falha("integrante já está em um projeto")
			if r == rgm {
				return common.Erro(409, "você já faz parte de um projeto"), nil
			}
			return common.Erro(409, "o RGM "+r+" já faz parte de outro projeto"), nil
		}
	}

	p, err := salvarProjeto(ctx, programaID, NovoProjeto{Nome: nome, Descricao: descricao, Integrantes: grupo})
	if err != nil {
		log.Printf("POST /meu-pfc: %v", err)
		return common.Erro(500, "falha ao criar projeto"), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "projeto.criado_pelo_aluno", Resultado: "sucesso",
		Recurso:  &auditoria.Recurso{Tipo: "projeto", ID: p.ID},
		Detalhes: map[string]any{"programaId": programaID, "nome": p.Nome, "integrantes": grupo},
	}, req)

	return common.JSON(201, p), nil
}
