package referencia

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// As referências moram na mesma partição do projeto:
//
//	PK = PROJECT#<projetoId>   SK = REF#<id>
//
// Um Query na PK com begins_with(SK, "REF#") lista tudo sem Scan nem GSI, e
// o id derivado do DOI (ver idDaReferencia) impede duplicata no projeto.

var errJaExiste = errors.New("referência já cadastrada no projeto")

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

	_, err = common.DynamoDB().PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(common.Tabela),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(SK)"),
	})
	if common.FalhouCondicao(err) {
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
	itens, err := common.QueryNaParticao[item](ctx, chaveProjeto(projetoID), "REF#")
	if err != nil {
		return nil, err
	}
	referencias := make([]Referencia, 0, len(itens))
	for _, it := range itens {
		referencias = append(referencias, toReferencia(it))
	}

	sort.Slice(referencias, func(a, b int) bool {
		return strings.ToUpper(referencias[a].ABNT) < strings.ToUpper(referencias[b].ABNT)
	})
	return referencias, nil
}

func remover(ctx context.Context, projetoID, referenciaID string) error {
	_, err := common.DynamoDB().DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(common.Tabela),
		Key:                 common.Chave(chaveProjeto(projetoID), "REF#"+referenciaID),
		ConditionExpression: aws.String("attribute_exists(SK)"),
	})
	if common.FalhouCondicao(err) {
		return errNaoEncontrado
	}
	return err
}
