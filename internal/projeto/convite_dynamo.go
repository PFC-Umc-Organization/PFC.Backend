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
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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

func chaveConvite(projetoID, rgm string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "PROJECT#" + projetoID},
		"SK": &types.AttributeValueMemberS{Value: "CONVITE#" + rgm},
	}
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

	_, err = ddb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(SK)"),
	})
	var condicao *types.ConditionalCheckFailedException
	if errors.As(err, &condicao) {
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
	out, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       chaveConvite(projetoID, rgm),
	})
	if err != nil {
		return Convite{}, false, err
	}
	if out.Item == nil {
		return Convite{}, false, nil
	}

	var it itemConvite
	if err := attributevalue.UnmarshalMap(out.Item, &it); err != nil {
		return Convite{}, false, err
	}
	return toConvite(it), true, nil
}

// convitesDoProjeto lista os convites pendentes do grupo.
func convitesDoProjeto(ctx context.Context, projetoID string) ([]Convite, error) {
	return queryConvites(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :c)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "PROJECT#" + projetoID},
			":c":  &types.AttributeValueMemberS{Value: "CONVITE#"},
		},
	})
}

// convitesDoRGM lista os convites que o aluno recebeu (de qualquer grupo).
func convitesDoRGM(ctx context.Context, rgm string) ([]Convite, error) {
	return queryConvites(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String(gsiName),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "CONVITE#" + rgm},
		},
	})
}

// queryConvites pagina até o fim: um Query devolve no máximo 1 MB por vez.
func queryConvites(ctx context.Context, in *dynamodb.QueryInput) ([]Convite, error) {
	convites := []Convite{}
	for {
		out, err := ddb.Query(ctx, in)
		if err != nil {
			return nil, err
		}
		for _, i := range out.Items {
			var it itemConvite
			if err := attributevalue.UnmarshalMap(i, &it); err != nil {
				continue
			}
			convites = append(convites, toConvite(it))
		}
		if out.LastEvaluatedKey == nil {
			return convites, nil
		}
		in.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

// entrarNoGrupo põe o RGM em integrantes ao aceitar o convite. A condição é
// conferida pelo DynamoDB no mesmo instante da escrita (atômica): um duplo
// clique não põe o aluno duas vezes no grupo, e dois aceites simultâneos
// não passam do limite. errNaoPodeEntrar se a condição falhar.
func entrarNoGrupo(ctx context.Context, projetoID, rgm string) error {
	_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PROJECT#" + projetoID},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		UpdateExpression:    aws.String("SET integrantes = list_append(integrantes, :novo)"),
		ConditionExpression: aws.String("attribute_exists(PK) AND NOT contains(integrantes, :rgm) AND size(integrantes) < :max"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":novo": &types.AttributeValueMemberL{Value: []types.AttributeValue{
				&types.AttributeValueMemberS{Value: rgm},
			}},
			":rgm": &types.AttributeValueMemberS{Value: rgm},
			":max": &types.AttributeValueMemberN{Value: fmt.Sprint(maxIntegrantes)},
		},
	})
	var condicao *types.ConditionalCheckFailedException
	if errors.As(err, &condicao) {
		return errNaoPodeEntrar
	}
	return err
}

// apagarConvite remove o convite. errConviteNaoExiste se ele não existia.
func apagarConvite(ctx context.Context, projetoID, rgm string) error {
	_, err := ddb.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(tableName),
		Key:                 chaveConvite(projetoID, rgm),
		ConditionExpression: aws.String("attribute_exists(SK)"),
	})
	var condicao *types.ConditionalCheckFailedException
	if errors.As(err, &condicao) {
		return errConviteNaoExiste
	}
	return err
}
