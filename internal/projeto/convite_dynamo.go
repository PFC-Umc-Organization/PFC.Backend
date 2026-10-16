package projeto

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// Modelagem (mesma tabela single-table dos demais domínios):
//
//	convite  PK=PROJECT#<projetoId>  SK=CONVITE#<rgm>  GSI1PK=CONVITE#<rgm>  GSI1SK=PROJECT#<projetoId>
//
// O convite fica na partição do projeto (convites do grupo = Query na PK) e
// o GSI1 lista os convites que um aluno recebeu, sem Scan. A SK por RGM faz
// o próprio DynamoDB barrar convite repetido pro mesmo aluno no mesmo grupo.
// Só existe convite pendente: aceitou, recusou ou foi cancelado, é apagado.

var (
	errConviteJaExiste  = errors.New("convite já enviado para este aluno")
	errConviteNaoExiste = errors.New("convite não encontrado")
	errNaoPodeEntrar    = errors.New("grupo cheio ou aluno já é integrante")
)

type itemConvite struct {
	PK           string `dynamodbav:"PK"`
	SK           string `dynamodbav:"SK"`
	GSI1PK       string `dynamodbav:"GSI1PK"`
	GSI1SK       string `dynamodbav:"GSI1SK"`
	ProjetoNome  string `dynamodbav:"projetoNome"`
	ConvidadoPor string `dynamodbav:"convidadoPor"`
	CriadoEm     string `dynamodbav:"criadoEm"`
}

func chaveConvite(projetoID, rgm string) common.Item {
	return common.Chave("PROJECT#"+projetoID, "CONVITE#"+rgm)
}

func toConvite(it itemConvite) Convite {
	return Convite{
		ProjetoID:    strings.TrimPrefix(it.PK, "PROJECT#"),
		ProjetoNome:  it.ProjetoNome,
		RGM:          strings.TrimPrefix(it.SK, "CONVITE#"),
		ConvidadoPor: it.ConvidadoPor,
		CriadoEm:     it.CriadoEm,
	}
}

// salvarConvite grava o convite pendente. errConviteJaExiste se o aluno já
// tem convite deste grupo.
func salvarConvite(ctx context.Context, projetoID, projetoNome, rgm, convidadoPor string) (Convite, error) {
	it := itemConvite{
		PK:           "PROJECT#" + projetoID,
		SK:           "CONVITE#" + rgm,
		GSI1PK:       "CONVITE#" + rgm,
		GSI1SK:       "PROJECT#" + projetoID,
		ProjetoNome:  projetoNome,
		ConvidadoPor: convidadoPor,
		CriadoEm:     time.Now().UTC().Format(time.RFC3339),
	}

	av, err := attributevalue.MarshalMap(it)
	if err != nil {
		return Convite{}, fmt.Errorf("falha ao serializar convite: %w", err)
	}

	_, err = common.DynamoDB().PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(common.Tabela),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(SK)"),
	})
	if common.FalhouCondicao(err) {
		return Convite{}, errConviteJaExiste
	}
	if err != nil {
		return Convite{}, err
	}
	return toConvite(it), nil
}

// buscarConvite devolve o convite do aluno pra este projeto. ok=false se
// não houver.
func buscarConvite(ctx context.Context, projetoID, rgm string) (c Convite, ok bool, err error) {
	it, ok, err := common.Buscar[itemConvite](ctx, chaveConvite(projetoID, rgm))
	if err != nil || !ok {
		return Convite{}, false, err
	}
	return toConvite(it), true, nil
}

// convitesDoProjeto lista os convites pendentes do grupo.
func convitesDoProjeto(ctx context.Context, projetoID string) ([]Convite, error) {
	return paraConvites(common.QueryNaParticao[itemConvite](ctx, "PROJECT#"+projetoID, "CONVITE#"))
}

// convitesDoRGM lista os convites que o aluno recebeu (de qualquer grupo).
func convitesDoRGM(ctx context.Context, rgm string) ([]Convite, error) {
	return paraConvites(common.QueryNoIndice[itemConvite](ctx, "CONVITE#"+rgm))
}

func paraConvites(itens []itemConvite, err error) ([]Convite, error) {
	if err != nil {
		return nil, err
	}
	convites := make([]Convite, 0, len(itens))
	for _, it := range itens {
		convites = append(convites, toConvite(it))
	}
	return convites, nil
}

// entrarNoGrupo põe o RGM em integrantes ao aceitar o convite, com as
// mesmas garantias atômicas de adicionarIntegrante: um duplo clique não põe
// o aluno duas vezes no grupo e dois aceites simultâneos não passam do
// limite (errNaoPodeEntrar); aluno que entrou em outro grupo no meio-tempo
// recebe errAlunoJaTemProjeto.
func entrarNoGrupo(ctx context.Context, projetoID, rgm string, atual []string) error {
	err := adicionarIntegrante(ctx, projetoID, rgm, atual)
	if errors.Is(err, errGrupoMudou) {
		return errNaoPodeEntrar
	}
	return err
}

// apagarConvite remove o convite. errConviteNaoExiste se ele não existia.
func apagarConvite(ctx context.Context, projetoID, rgm string) error {
	_, err := common.DynamoDB().DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(common.Tabela),
		Key:                 chaveConvite(projetoID, rgm),
		ConditionExpression: aws.String("attribute_exists(SK)"),
	})
	if common.FalhouCondicao(err) {
		return errConviteNaoExiste
	}
	return err
}
