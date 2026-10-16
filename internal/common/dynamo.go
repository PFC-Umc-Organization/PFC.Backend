package common

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

type Item = dynamo.Item

type ErroDeCondicao = dynamo.ConditionError

const TamanhoLote = dynamo.BatchWriteSize

func S(v string) types.AttributeValue { return dynamo.S(v) }

func N(v int) types.AttributeValue { return dynamo.N(v) }

func Chave(pk, sk string) Item { return dynamo.Key(pk, sk) }

func Texto(it Item, nome string) string { return dynamo.String(it, nome) }

func QueryTudo(ctx context.Context, in *dynamodb.QueryInput) ([]Item, error) {
	return table().QueryAll(ctx, in)
}

func QueryComo[T any](ctx context.Context, in *dynamodb.QueryInput) ([]T, error) {
	return dynamo.Query[T](ctx, table(), in)
}

func QueryNoIndice[T any](ctx context.Context, gsi1pk string) ([]T, error) {
	return dynamo.QueryIndex[T](ctx, table(), gsi1pk)
}

func QueryNaParticao[T any](ctx context.Context, pk, prefixoSK string) ([]T, error) {
	return dynamo.QueryPartition[T](ctx, table(), pk, prefixoSK)
}

func Buscar[T any](ctx context.Context, chave Item, projecao ...string) (T, bool, error) {
	return dynamo.Get[T](ctx, table(), chave, projecao...)
}

func Existe(ctx context.Context, chave Item) (bool, error) { return table().Exists(ctx, chave) }

func BuscarVarios(ctx context.Context, chaves []Item, projecao ...string) ([]Item, error) {
	return table().BatchGet(ctx, chaves, projecao...)
}

func GravarLote(ctx context.Context, pedidos []types.WriteRequest) error {
	return table().BatchWrite(ctx, pedidos)
}

func GravarEmLotes(ctx context.Context, pedidos []types.WriteRequest) error {
	return table().BatchWrite(ctx, pedidos)
}

func ApagarChaves(ctx context.Context, chaves []Item) error { return table().DeleteKeys(ctx, chaves) }

func Transacao(ctx context.Context, itens ...types.TransactWriteItem) error {
	return table().Transact(ctx, itens...)
}

func FalhouItem(err error, i int) bool { return dynamo.FailedAt(err, i) }

func FalhouCondicao(err error) bool { return dynamo.ConditionFailed(err) }

func Put(item Item, condicao string, valores Item) types.TransactWriteItem {
	return table().Put(item, condicao, valores)
}

func Delete(chave Item, condicao string, valores Item) types.TransactWriteItem {
	return table().Delete(chave, condicao, valores)
}

func Update(chave Item, expressao, condicao string, valores Item) types.TransactWriteItem {
	return table().Update(chave, expressao, condicao, valores)
}

func Conferir(chave Item, condicao string, valores Item) types.TransactWriteItem {
	return table().Check(chave, condicao, valores)
}
