package atividade

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

// Modelagem (mesma tabela single-table dos demais domínios):
//
//	atividade  PK=ACTIVITY#<id>  SK=PROFILE        GSI1PK=ACTIVITIES      GSI1SK=ACTIVITY#<id>
//	entrega    PK=PROJECT#<pid>  SK=ENTREGA#<aid>  GSI1PK=ENTREGAS#<aid>  GSI1SK=PROJECT#<pid>
//
// A entrega fica na partição do projeto (listar as do grupo = Query na PK) e
// o GSI1 permite listar as de uma atividade (visão do professor).

var (
	tableName = os.Getenv("TABLE_NAME")
	gsiName   = os.Getenv("GSI_NAME")
	ddb       *dynamodb.Client
)

var (
	errNaoEncontrada  = errors.New("atividade não encontrada")
	errCampoNaoExiste = errors.New("campo não encontrado")
	errUltimoCampo    = errors.New("a atividade precisa de ao menos um campo de entrega")
	errLimiteDeCampos = errors.New("limite de campos da atividade atingido")
)

func init() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(fmt.Sprintf("falha ao carregar config AWS: %v", err))
	}
	ddb = dynamodb.NewFromConfig(cfg)
}

type item struct {
	PK          string  `dynamodbav:"PK"`
	SK          string  `dynamodbav:"SK"`
	GSI1PK      string  `dynamodbav:"GSI1PK"`
	GSI1SK      string  `dynamodbav:"GSI1SK"`
	Titulo      string  `dynamodbav:"titulo"`
	Descricao   string  `dynamodbav:"descricao"`
	Prazo       string  `dynamodbav:"prazo"`
	PublicadaEm string  `dynamodbav:"publicadaEm"`
	Campos      []Campo `dynamodbav:"campos"`
}

type itemEntrega struct {
	PK          string            `dynamodbav:"PK"`
	SK          string            `dynamodbav:"SK"`
	GSI1PK      string            `dynamodbav:"GSI1PK"`
	GSI1SK      string            `dynamodbav:"GSI1SK"`
	EntregueEm  string            `dynamodbav:"entregueEm"`
	EntreguePor string            `dynamodbav:"entreguePor"`
	Respostas   map[string]string `dynamodbav:"respostas"`
}

func str(v string) types.AttributeValue { return &types.AttributeValueMemberS{Value: v} }

func chaveAtividade(id string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"PK": str("ACTIVITY#" + id), "SK": str("PROFILE")}
}

func toAtividade(it item) Atividade {
	campos := it.Campos
	if campos == nil {
		campos = []Campo{}
	}
	return Atividade{
		ID:          strings.TrimPrefix(it.PK, "ACTIVITY#"),
		Titulo:      it.Titulo,
		Descricao:   it.Descricao,
		Prazo:       it.Prazo,
		PublicadaEm: it.PublicadaEm,
		Campos:      campos,
	}
}

func toEntrega(it itemEntrega) Entrega {
	respostas := it.Respostas
	if respostas == nil {
		respostas = map[string]string{}
	}
	return Entrega{
		AtividadeID: strings.TrimPrefix(it.SK, "ENTREGA#"),
		ProjetoID:   strings.TrimPrefix(it.PK, "PROJECT#"),
		EntregueEm:  it.EntregueEm,
		EntreguePor: it.EntreguePor,
		Respostas:   respostas,
	}
}

// salvar cria a atividade já com o campo de entrega padrão.
func salvar(ctx context.Context, d dadosAtividade) (Atividade, error) {
	id := uuid.NewString()
	it := item{
		PK:          "ACTIVITY#" + id,
		SK:          "PROFILE",
		GSI1PK:      "ACTIVITIES",
		GSI1SK:      "ACTIVITY#" + id,
		Titulo:      d.Titulo,
		Descricao:   d.Descricao,
		Prazo:       d.Prazo,
		PublicadaEm: time.Now().UTC().Format(time.RFC3339),
		Campos: []Campo{{
			ID: uuid.NewString(), Rotulo: "Arquivo da entrega", Tipo: CampoArquivo, Obrigatorio: true,
		}},
	}
	av, err := attributevalue.MarshalMap(it)
	if err != nil {
		return Atividade{}, fmt.Errorf("falha ao serializar atividade: %w", err)
	}
	if _, err := ddb.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(tableName), Item: av}); err != nil {
		return Atividade{}, err
	}
	return toAtividade(it), nil
}

// listar devolve as atividades ordenadas por prazo.
func listar(ctx context.Context) ([]Atividade, error) {
	atividades := []Atividade{}
	var chave map[string]types.AttributeValue
	for {
		out, err := ddb.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(tableName),
			IndexName:                 aws.String(gsiName),
			KeyConditionExpression:    aws.String("GSI1PK = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{":pk": str("ACTIVITIES")},
			ExclusiveStartKey:         chave,
		})
		if err != nil {
			return nil, err
		}
		for _, i := range out.Items {
			var it item
			if err := attributevalue.UnmarshalMap(i, &it); err != nil {
				continue
			}
			atividades = append(atividades, toAtividade(it))
		}
		if out.LastEvaluatedKey == nil {
			break
		}
		chave = out.LastEvaluatedKey
	}
	sort.Slice(atividades, func(a, b int) bool { return atividades[a].Prazo < atividades[b].Prazo })
	return atividades, nil
}

// buscar devolve a atividade pelo id. ok=false quando não existe.
func buscar(ctx context.Context, id string) (a Atividade, ok bool, err error) {
	out, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(tableName), Key: chaveAtividade(id)})
	if err != nil {
		return Atividade{}, false, err
	}
	if out.Item == nil {
		return Atividade{}, false, nil
	}
	var it item
	if err := attributevalue.UnmarshalMap(out.Item, &it); err != nil {
		return Atividade{}, false, err
	}
	return toAtividade(it), true, nil
}

func atualizar(ctx context.Context, id string, d dadosAtividade) error {
	_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(tableName),
		Key:                 chaveAtividade(id),
		UpdateExpression:    aws.String("SET titulo = :t, descricao = :d, prazo = :p"),
		ConditionExpression: aws.String("attribute_exists(PK)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":t": str(d.Titulo), ":d": str(d.Descricao), ":p": str(d.Prazo),
		},
	})
	if isCondicao(err) {
		return errNaoEncontrada
	}
	return err
}

// deletar remove a atividade e as entregas dela.
func deletar(ctx context.Context, id string) error {
	_, err := ddb.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(tableName),
		Key:                 chaveAtividade(id),
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})
	if isCondicao(err) {
		return errNaoEncontrada
	}
	if err != nil {
		return err
	}
	return apagarEntregas(ctx, id)
}

func apagarEntregas(ctx context.Context, atividadeID string) error {
	var pedidos []types.WriteRequest
	var inicio map[string]types.AttributeValue
	for {
		out, err := ddb.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(tableName),
			IndexName:                 aws.String(gsiName),
			KeyConditionExpression:    aws.String("GSI1PK = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{":pk": str("ENTREGAS#" + atividadeID)},
			ProjectionExpression:      aws.String("PK, SK"),
			ExclusiveStartKey:         inicio,
		})
		if err != nil {
			return err
		}
		for _, i := range out.Items {
			pedidos = append(pedidos, types.WriteRequest{DeleteRequest: &types.DeleteRequest{
				Key: map[string]types.AttributeValue{"PK": i["PK"], "SK": i["SK"]},
			}})
		}
		if out.LastEvaluatedKey == nil {
			break
		}
		inicio = out.LastEvaluatedKey
	}

	for len(pedidos) > 0 {
		n := len(pedidos)
		if n > 25 {
			n = 25
		}
		lote := pedidos[:n]
		pedidos = pedidos[n:]
		for len(lote) > 0 {
			out, err := ddb.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
				RequestItems: map[string][]types.WriteRequest{tableName: lote},
			})
			if err != nil {
				return err
			}
			lote = out.UnprocessedItems[tableName]
		}
	}
	return nil
}

func adicionarCampo(ctx context.Context, atividadeID string, nc NovoCampo) (Campo, error) {
	campo := Campo{ID: uuid.NewString(), Rotulo: nc.Rotulo, Tipo: nc.Tipo, Obrigatorio: nc.Obrigatorio}
	av, err := attributevalue.Marshal(campo)
	if err != nil {
		return Campo{}, err
	}
	_, err = ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(tableName),
		Key:                 chaveAtividade(atividadeID),
		UpdateExpression:    aws.String("SET campos = list_append(campos, :c)"),
		ConditionExpression: aws.String("attribute_exists(PK) AND size(campos) < :max"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":c":   &types.AttributeValueMemberL{Value: []types.AttributeValue{av}},
			":max": &types.AttributeValueMemberN{Value: fmt.Sprint(maxCampos)},
		},
	})
	if isCondicao(err) {
		if _, ok, e := buscar(ctx, atividadeID); e == nil && ok {
			return Campo{}, errLimiteDeCampos
		}
		return Campo{}, errNaoEncontrada
	}
	if err != nil {
		return Campo{}, err
	}
	return campo, nil
}

// removerCampo tira o campo da lista, recusando o último. A condição no
// UpdateItem (tamanho > 1 e id na posição) torna a operação atômica.
func removerCampo(ctx context.Context, atividadeID, campoID string) error {
	a, ok, err := buscar(ctx, atividadeID)
	if err != nil {
		return err
	}
	if !ok {
		return errNaoEncontrada
	}
	pos := -1
	for i, c := range a.Campos {
		if c.ID == campoID {
			pos = i
			break
		}
	}
	if pos < 0 {
		return errCampoNaoExiste
	}
	if len(a.Campos) <= 1 {
		return errUltimoCampo
	}

	_, err = ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(tableName),
		Key:                 chaveAtividade(atividadeID),
		UpdateExpression:    aws.String(fmt.Sprintf("REMOVE campos[%d]", pos)),
		ConditionExpression: aws.String(fmt.Sprintf("size(campos) > :um AND campos[%d].id = :id", pos)),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":um": &types.AttributeValueMemberN{Value: "1"},
			":id": str(campoID),
		},
	})
	if isCondicao(err) {
		// Mudou entre a leitura e a escrita — o cliente tenta de novo.
		return errCampoNaoExiste
	}
	return err
}

func salvarEntrega(ctx context.Context, projetoID, atividadeID, por string, respostas map[string]string) (Entrega, error) {
	it := itemEntrega{
		PK:          "PROJECT#" + projetoID,
		SK:          "ENTREGA#" + atividadeID,
		GSI1PK:      "ENTREGAS#" + atividadeID,
		GSI1SK:      "PROJECT#" + projetoID,
		EntregueEm:  time.Now().UTC().Format(time.RFC3339),
		EntreguePor: por,
		Respostas:   respostas,
	}
	av, err := attributevalue.MarshalMap(it)
	if err != nil {
		return Entrega{}, err
	}
	if _, err := ddb.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(tableName), Item: av}); err != nil {
		return Entrega{}, err
	}
	return toEntrega(it), nil
}

func entregasDoProjeto(ctx context.Context, projetoID string) ([]Entrega, error) {
	return queryEntregas(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :e)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": str("PROJECT#" + projetoID), ":e": str("ENTREGA#"),
		},
	})
}

func entregasDaAtividade(ctx context.Context, atividadeID string) ([]Entrega, error) {
	return queryEntregas(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(tableName),
		IndexName:                 aws.String(gsiName),
		KeyConditionExpression:    aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":pk": str("ENTREGAS#" + atividadeID)},
	})
}

func queryEntregas(ctx context.Context, in *dynamodb.QueryInput) ([]Entrega, error) {
	entregas := []Entrega{}
	for {
		out, err := ddb.Query(ctx, in)
		if err != nil {
			return nil, err
		}
		for _, i := range out.Items {
			var it itemEntrega
			if err := attributevalue.UnmarshalMap(i, &it); err != nil {
				continue
			}
			entregas = append(entregas, toEntrega(it))
		}
		if out.LastEvaluatedKey == nil {
			return entregas, nil
		}
		in.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func isCondicao(err error) bool {
	var c *types.ConditionalCheckFailedException
	return errors.As(err, &c)
}
