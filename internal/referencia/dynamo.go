package referencia

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// As referências moram na mesma partição do projeto:
//
//	PK = PROJECT#<projetoId>   SK = REF#<id>
//
// Um Query na PK com begins_with(SK, "REF#") lista tudo sem Scan nem GSI, e
// o id derivado do DOI (ver idDaReferencia) impede duplicata no projeto.

var (
	tableName = os.Getenv("TABLE_NAME")
	ddb       *dynamodb.Client
)

var errJaExiste = errors.New("referência já cadastrada no projeto")

func init() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(fmt.Sprintf("falha ao carregar config AWS: %v", err))
	}
	ddb = dynamodb.NewFromConfig(cfg)
}

type item struct {
	PK              string `dynamodbav:"PK"`
	SK              string `dynamodbav:"SK"`
	DOI             string `dynamodbav:"doi"`
	Titulo          string `dynamodbav:"titulo"`
	Ano             int    `dynamodbav:"ano,omitempty"`
	ABNT            string `dynamodbav:"abnt"`
	ABNTHTML        string `dynamodbav:"abntHtml"`
	AdicionadaPorID string `dynamodbav:"adicionadaPorId"`
	AdicionadaPor   string `dynamodbav:"adicionadaPor"`
	AdicionadaEm    string `dynamodbav:"adicionadaEm"`
}

func chaveProjeto(projetoID string) string {
	return "PROJECT#" + projetoID
}

func toReferencia(it item) Referencia {
	return Referencia{
		ID:            strings.TrimPrefix(it.SK, "REF#"),
		ProjetoID:     strings.TrimPrefix(it.PK, "PROJECT#"),
		DOI:           it.DOI,
		Titulo:        it.Titulo,
		Ano:           it.Ano,
		ABNT:          it.ABNT,
		ABNTHTML:      it.ABNTHTML,
		AdicionadaPor: it.AdicionadaPor,
		AdicionadaEm:  it.AdicionadaEm,
	}
}

func salvar(ctx context.Context, projetoID string, r Referencia, autorID string) (Referencia, error) {
	it := item{
		PK:              chaveProjeto(projetoID),
		SK:              "REF#" + r.ID,
		DOI:             r.DOI,
		Titulo:          r.Titulo,
		Ano:             r.Ano,
		ABNT:            r.ABNT,
		ABNTHTML:        r.ABNTHTML,
		AdicionadaPorID: autorID,
		AdicionadaPor:   r.AdicionadaPor,
		AdicionadaEm:    r.AdicionadaEm,
	}

	av, err := attributevalue.MarshalMap(it)
	if err != nil {
		return Referencia{}, fmt.Errorf("falha ao serializar referência: %w", err)
	}

	_, err = ddb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(SK)"),
	})
	var condicao *types.ConditionalCheckFailedException
	if errors.As(err, &condicao) {
		return Referencia{}, errJaExiste
	}
	if err != nil {
		return Referencia{}, err
	}
	return toReferencia(it), nil
}

// listar devolve as referências do projeto em ordem alfabética — é a ordem
// da lista de referências no trabalho (NBR 6023, sistema autor-data).
func listar(ctx context.Context, projetoID string) ([]Referencia, error) {
	referencias := []Referencia{}

	// Pagina até o fim: um Query devolve no máximo 1 MB por chamada.
	var inicio map[string]types.AttributeValue
	for {
		out, err := ddb.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(tableName),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :ref)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":  &types.AttributeValueMemberS{Value: chaveProjeto(projetoID)},
				":ref": &types.AttributeValueMemberS{Value: "REF#"},
			},
			ExclusiveStartKey: inicio,
		})
		if err != nil {
			return nil, err
		}

		for _, i := range out.Items {
			var it item
			if err := attributevalue.UnmarshalMap(i, &it); err != nil {
				continue
			}
			referencias = append(referencias, toReferencia(it))
		}

		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		inicio = out.LastEvaluatedKey
	}

	sort.Slice(referencias, func(a, b int) bool {
		return strings.ToUpper(referencias[a].ABNT) < strings.ToUpper(referencias[b].ABNT)
	})
	return referencias, nil
}

func remover(ctx context.Context, projetoID, referenciaID string) error {
	_, err := ddb.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: chaveProjeto(projetoID)},
			"SK": &types.AttributeValueMemberS{Value: "REF#" + referenciaID},
		},
		ConditionExpression: aws.String("attribute_exists(SK)"),
	})
	var condicao *types.ConditionalCheckFailedException
	if errors.As(err, &condicao) {
		return errNaoEncontrado
	}
	return err
}
