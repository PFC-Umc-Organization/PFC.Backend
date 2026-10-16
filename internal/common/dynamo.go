package common

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Item é um item cru do DynamoDB.
type Item = map[string]types.AttributeValue

// S monta um atributo string.
func S(v string) types.AttributeValue { return &types.AttributeValueMemberS{Value: v} }

// N monta um atributo numérico.
func N(v int) types.AttributeValue { return &types.AttributeValueMemberN{Value: fmt.Sprint(v)} }

// Chave monta a chave primária (PK, SK) de um item.
func Chave(pk, sk string) Item {
	return Item{"PK": S(pk), "SK": S(sk)}
}

// Texto lê um atributo string do item cru ("" se ausente ou de outro tipo).
func Texto(it Item, nome string) string {
	if v, ok := it[nome].(*types.AttributeValueMemberS); ok {
		return v.Value
	}
	return ""
}

// QueryTudo pagina o Query até o fim. Um Query lê no máximo 1 MB por
// chamada — sem paginar, a lista vem cortada sem erro nenhum.
func QueryTudo(ctx context.Context, in *dynamodb.QueryInput) ([]Item, error) {
	var itens []Item
	for {
		out, err := DynamoDB().Query(ctx, in)
		if err != nil {
			return nil, err
		}
		itens = append(itens, out.Items...)
		if len(out.LastEvaluatedKey) == 0 {
			return itens, nil
		}
		in.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

// QueryComo pagina o Query e converte cada item em T. Sempre devolve lista
// (nunca nil), pra virar [] e não null no JSON.
func QueryComo[T any](ctx context.Context, in *dynamodb.QueryInput) ([]T, error) {
	itens, err := QueryTudo(ctx, in)
	if err != nil {
		return nil, err
	}
	lista := make([]T, 0, len(itens))
	if err := attributevalue.UnmarshalListOfMaps(itens, &lista); err != nil {
		return nil, fmt.Errorf("item em formato inesperado: %w", err)
	}
	return lista, nil
}

// QueryNoIndice pagina um Query no GSI1 pela partição GSI1PK.
func QueryNoIndice[T any](ctx context.Context, gsi1pk string) ([]T, error) {
	return QueryComo[T](ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(Tabela),
		IndexName:                 aws.String(Indice),
		KeyConditionExpression:    aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: Item{":pk": S(gsi1pk)},
	})
}

// QueryNaParticao pagina um Query na PK, só com as SKs que começam com o
// prefixo (ex.: os convites de um projeto).
func QueryNaParticao[T any](ctx context.Context, pk, prefixoSK string) ([]T, error) {
	return QueryComo[T](ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(Tabela),
		KeyConditionExpression:    aws.String("PK = :pk AND begins_with(SK, :sk)"),
		ExpressionAttributeValues: Item{":pk": S(pk), ":sk": S(prefixoSK)},
	})
}

// Buscar faz GetItem e converte o item em T. ok=false quando não existe.
// projecao (opcional) limita os atributos lidos — útil quando só interessa
// saber se o item existe ou ler um campo.
func Buscar[T any](ctx context.Context, chave Item, projecao ...string) (v T, ok bool, err error) {
	in := &dynamodb.GetItemInput{TableName: aws.String(Tabela), Key: chave}
	if len(projecao) > 0 {
		in.ProjectionExpression = aws.String(projecaoDe(projecao))
	}
	out, err := DynamoDB().GetItem(ctx, in)
	if err != nil || out.Item == nil {
		return v, false, err
	}
	if err := attributevalue.UnmarshalMap(out.Item, &v); err != nil {
		return v, false, fmt.Errorf("item em formato inesperado: %w", err)
	}
	return v, true, nil
}

// Existe diz se há item com a chave, lendo só a PK.
func Existe(ctx context.Context, chave Item) (bool, error) {
	_, ok, err := Buscar[struct{}](ctx, chave, "PK")
	return ok, err
}

func projecaoDe(atributos []string) string {
	p := atributos[0]
	for _, a := range atributos[1:] {
		p += ", " + a
	}
	return p
}

// tamanhoLoteLeitura é o máximo de chaves que o BatchGetItem aceita.
const tamanhoLoteLeitura = 100

// BuscarVarios lê vários itens por chave com BatchGetItem (em lotes de 100,
// reenviando UnprocessedKeys). Uma ida ao banco no lugar de N GetItem. Itens
// inexistentes simplesmente não voltam; a ordem não é garantida.
func BuscarVarios(ctx context.Context, chaves []Item, projecao ...string) ([]Item, error) {
	var itens []Item
	for inicio := 0; inicio < len(chaves); inicio += tamanhoLoteLeitura {
		pedido := types.KeysAndAttributes{Keys: chaves[inicio:min(inicio+tamanhoLoteLeitura, len(chaves))]}
		if len(projecao) > 0 {
			pedido.ProjectionExpression = aws.String(projecaoDe(projecao))
		}
		pendentes := map[string]types.KeysAndAttributes{Tabela: pedido}
		espera := esperaInicial
		for tentativa := 0; len(pendentes) > 0; tentativa++ {
			if tentativa == tentativasLote {
				return nil, fmt.Errorf("chaves não lidas após %d tentativas", tentativasLote)
			}
			if tentativa > 0 {
				if err := aguardar(ctx, espera); err != nil {
					return nil, err
				}
				espera *= 2
			}
			out, err := DynamoDB().BatchGetItem(ctx, &dynamodb.BatchGetItemInput{RequestItems: pendentes})
			if err != nil {
				return nil, err
			}
			itens = append(itens, out.Responses[Tabela]...)
			pendentes = out.UnprocessedKeys
		}
	}
	return itens, nil
}

// TamanhoLote é o máximo de pedidos que o BatchWriteItem aceita por chamada.
const TamanhoLote = 25

const (
	tentativasLote = 5
	esperaInicial  = 50 * time.Millisecond
)

func aguardar(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// GravarLote manda até 25 pedidos num BatchWriteItem e reenvia os
// UnprocessedItems (com espera crescente) até acabarem. Sem o reenvio, o
// DynamoDB pode descartar parte do lote sob throttling sem devolver erro.
func GravarLote(ctx context.Context, pedidos []types.WriteRequest) error {
	espera := esperaInicial
	for tentativa := 0; len(pedidos) > 0; tentativa++ {
		if tentativa == tentativasLote {
			return fmt.Errorf("%d itens não processados após %d tentativas", len(pedidos), tentativasLote)
		}
		if tentativa > 0 {
			if err := aguardar(ctx, espera); err != nil {
				return err
			}
			espera *= 2
		}
		out, err := DynamoDB().BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{Tabela: pedidos},
		})
		if err != nil {
			return err
		}
		pedidos = out.UnprocessedItems[Tabela]
	}
	return nil
}

// GravarEmLotes divide os pedidos em lotes de 25 e grava cada um com
// GravarLote.
func GravarEmLotes(ctx context.Context, pedidos []types.WriteRequest) error {
	for inicio := 0; inicio < len(pedidos); inicio += TamanhoLote {
		if err := GravarLote(ctx, pedidos[inicio:min(inicio+TamanhoLote, len(pedidos))]); err != nil {
			return err
		}
	}
	return nil
}

// ApagarChaves apaga os itens pelas chaves, em lotes.
func ApagarChaves(ctx context.Context, chaves []Item) error {
	pedidos := make([]types.WriteRequest, 0, len(chaves))
	for _, c := range chaves {
		pedidos = append(pedidos, types.WriteRequest{DeleteRequest: &types.DeleteRequest{Key: c}})
	}
	return GravarEmLotes(ctx, pedidos)
}

// Transacao grava os itens com TransactWriteItems: tudo ou nada, com as
// condições conferidas no mesmo instante. É o que garante as regras de
// unicidade (uma turma por curso/turno/período, um programa por turma, um
// projeto por aluno) mesmo com duas requisições ao mesmo tempo.
//
// Se alguma condição falhar, o erro é *ErroDeCondicao com as posições dos
// itens que falharam — quem chama traduz isso no erro de domínio.
func Transacao(ctx context.Context, itens ...types.TransactWriteItem) error {
	_, err := DynamoDB().TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: itens})
	var cancelada *types.TransactionCanceledException
	if errors.As(err, &cancelada) {
		var falhas []int
		for i, motivo := range cancelada.CancellationReasons {
			if aws.ToString(motivo.Code) == "ConditionalCheckFailed" {
				falhas = append(falhas, i)
			}
		}
		if len(falhas) > 0 {
			return &ErroDeCondicao{Posicoes: falhas, causa: err}
		}
	}
	return err
}

// ErroDeCondicao diz quais itens da transação tiveram a condição recusada.
type ErroDeCondicao struct {
	Posicoes []int
	causa    error
}

func (e *ErroDeCondicao) Error() string {
	return fmt.Sprintf("condição recusada nos itens %v da transação", e.Posicoes)
}

func (e *ErroDeCondicao) Unwrap() error { return e.causa }

// FalhouItem diz se o erro é de transação com a condição do item i recusada.
func FalhouItem(err error, i int) bool {
	var c *ErroDeCondicao
	if !errors.As(err, &c) {
		return false
	}
	for _, p := range c.Posicoes {
		if p == i {
			return true
		}
	}
	return false
}

// Put, Delete, Update e Conferir montam os itens de uma Transacao.

func Put(item Item, condicao string, valores Item) types.TransactWriteItem {
	p := &types.Put{TableName: aws.String(Tabela), Item: item}
	if condicao != "" {
		p.ConditionExpression = aws.String(condicao)
	}
	if len(valores) > 0 {
		p.ExpressionAttributeValues = valores
	}
	return types.TransactWriteItem{Put: p}
}

func Delete(chave Item, condicao string, valores Item) types.TransactWriteItem {
	d := &types.Delete{TableName: aws.String(Tabela), Key: chave}
	if condicao != "" {
		d.ConditionExpression = aws.String(condicao)
	}
	if len(valores) > 0 {
		d.ExpressionAttributeValues = valores
	}
	return types.TransactWriteItem{Delete: d}
}

func Update(chave Item, expressao, condicao string, valores Item) types.TransactWriteItem {
	u := &types.Update{TableName: aws.String(Tabela), Key: chave, UpdateExpression: aws.String(expressao)}
	if condicao != "" {
		u.ConditionExpression = aws.String(condicao)
	}
	if len(valores) > 0 {
		u.ExpressionAttributeValues = valores
	}
	return types.TransactWriteItem{Update: u}
}

func Conferir(chave Item, condicao string, valores Item) types.TransactWriteItem {
	c := &types.ConditionCheck{TableName: aws.String(Tabela), Key: chave, ConditionExpression: aws.String(condicao)}
	if len(valores) > 0 {
		c.ExpressionAttributeValues = valores
	}
	return types.TransactWriteItem{ConditionCheck: c}
}

// FalhouCondicao diz se o erro é de ConditionExpression não satisfeita —
// em geral "o item não existe" (attribute_exists) ou "já existe".
func FalhouCondicao(err error) bool {
	var c *types.ConditionalCheckFailedException
	return errors.As(err, &c)
}
