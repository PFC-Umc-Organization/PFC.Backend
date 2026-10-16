package atividade

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// Modelagem (mesma tabela single-table dos demais domínios):
//
//	atividade  PK=ACTIVITY#<id>  SK=PROFILE        GSI1PK=ACTIVITIES      GSI1SK=ACTIVITY#<id>
//	entrega    PK=PROJECT#<pid>  SK=ENTREGA#<aid>  GSI1PK=ENTREGAS#<aid>  GSI1SK=PROJECT#<pid>
//
// A entrega fica na partição do projeto (listar as do grupo = Query na PK) e
// o GSI1 permite listar as de uma atividade (visão do professor).

var (
	errNaoEncontrada    = errors.New("atividade não encontrada")
	errCampoNaoExiste   = errors.New("campo não encontrado")
	errUltimoCampo      = errors.New("a atividade precisa de ao menos um campo de entrega")
	errEntregaNaoExiste = errors.New("entrega não encontrada")
	errLimiteDeCampos   = errors.New("limite de campos da atividade atingido")
)

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

var str = common.S

func chaveAtividade(id string) common.Item { return common.Chave("ACTIVITY#"+id, "PROFILE") }

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

// salvar cria a atividade com os campos de entrega informados.
func salvar(ctx context.Context, d dadosAtividade, novos []NovoCampo) (Atividade, error) {
	id := uuid.NewString()
	campos := make([]Campo, 0, len(novos))
	for _, c := range novos {
		campos = append(campos, Campo{ID: uuid.NewString(), Rotulo: c.Rotulo, Tipo: c.Tipo, Obrigatorio: c.Obrigatorio})
	}
	it := item{
		PK:          "ACTIVITY#" + id,
		SK:          "PROFILE",
		GSI1PK:      "ACTIVITIES",
		GSI1SK:      "ACTIVITY#" + id,
		Titulo:      d.Titulo,
		Descricao:   d.Descricao,
		Prazo:       d.Prazo,
		PublicadaEm: time.Now().UTC().Format(time.RFC3339),
		Campos:      campos,
	}
	av, err := attributevalue.MarshalMap(it)
	if err != nil {
		return Atividade{}, fmt.Errorf("falha ao serializar atividade: %w", err)
	}
	if _, err := common.DynamoDB().PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(common.Tabela), Item: av}); err != nil {
		return Atividade{}, err
	}
	return toAtividade(it), nil
}

// listar devolve as atividades ordenadas por prazo.
func listar(ctx context.Context) ([]Atividade, error) {
	itens, err := common.QueryNoIndice[item](ctx, "ACTIVITIES")
	if err != nil {
		return nil, err
	}
	atividades := make([]Atividade, 0, len(itens))
	for _, it := range itens {
		atividades = append(atividades, toAtividade(it))
	}
	sort.Slice(atividades, func(a, b int) bool { return atividades[a].Prazo < atividades[b].Prazo })
	return atividades, nil
}

// buscar devolve a atividade pelo id. ok=false quando não existe.
func buscar(ctx context.Context, id string) (a Atividade, ok bool, err error) {
	it, ok, err := common.Buscar[item](ctx, chaveAtividade(id))
	if err != nil || !ok {
		return Atividade{}, false, err
	}
	return toAtividade(it), true, nil
}

// atualizar grava título, descrição e prazo e devolve a atividade já
// atualizada (ALL_NEW) — sem uma segunda leitura.
func atualizar(ctx context.Context, id string, d dadosAtividade) (Atividade, error) {
	out, err := common.DynamoDB().UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(common.Tabela),
		Key:                 chaveAtividade(id),
		UpdateExpression:    aws.String("SET titulo = :t, descricao = :d, prazo = :p"),
		ConditionExpression: aws.String("attribute_exists(PK)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":t": str(d.Titulo), ":d": str(d.Descricao), ":p": str(d.Prazo),
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if common.FalhouCondicao(err) {
		return Atividade{}, errNaoEncontrada
	}
	if err != nil {
		return Atividade{}, err
	}
	var it item
	if err := attributevalue.UnmarshalMap(out.Attributes, &it); err != nil {
		return Atividade{}, err
	}
	return toAtividade(it), nil
}

// deletar remove a atividade e as entregas dela.
func deletar(ctx context.Context, id string) error {
	_, err := common.DynamoDB().DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(common.Tabela),
		Key:                 chaveAtividade(id),
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})
	if common.FalhouCondicao(err) {
		return errNaoEncontrada
	}
	if err != nil {
		return err
	}
	return apagarEntregas(ctx, id)
}

func apagarEntregas(ctx context.Context, atividadeID string) error {
	itens, err := common.QueryTudo(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(common.Tabela),
		IndexName:                 aws.String(common.Indice),
		KeyConditionExpression:    aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":pk": str("ENTREGAS#" + atividadeID)},
		ProjectionExpression:      aws.String("PK, SK"),
	})
	if err != nil {
		return err
	}
	chaves := make([]common.Item, 0, len(itens))
	for _, i := range itens {
		chaves = append(chaves, common.Item{"PK": i["PK"], "SK": i["SK"]})
	}
	return common.ApagarChaves(ctx, chaves)
}

func adicionarCampo(ctx context.Context, atividadeID string, nc NovoCampo) (Campo, error) {
	campo := Campo{ID: uuid.NewString(), Rotulo: nc.Rotulo, Tipo: nc.Tipo, Obrigatorio: nc.Obrigatorio}
	av, err := attributevalue.Marshal(campo)
	if err != nil {
		return Campo{}, err
	}
	_, err = common.DynamoDB().UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(common.Tabela),
		Key:                 chaveAtividade(atividadeID),
		UpdateExpression:    aws.String("SET campos = list_append(campos, :c)"),
		ConditionExpression: aws.String("attribute_exists(PK) AND size(campos) < :max"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":c":   &types.AttributeValueMemberL{Value: []types.AttributeValue{av}},
			":max": common.N(maxCampos),
		},
		// Na recusa, o DynamoDB devolve o item como estava: dá pra saber se
		// faltou a atividade ou sobrou campo sem uma segunda leitura.
		ReturnValuesOnConditionCheckFailure: types.ReturnValuesOnConditionCheckFailureAllOld,
	})
	var recusa *types.ConditionalCheckFailedException
	if errors.As(err, &recusa) {
		if len(recusa.Item) > 0 {
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

	_, err = common.DynamoDB().UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(common.Tabela),
		Key:                 chaveAtividade(atividadeID),
		UpdateExpression:    aws.String(fmt.Sprintf("REMOVE campos[%d]", pos)),
		ConditionExpression: aws.String(fmt.Sprintf("size(campos) > :um AND campos[%d].id = :id", pos)),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":um": &types.AttributeValueMemberN{Value: "1"},
			":id": str(campoID),
		},
	})
	if common.FalhouCondicao(err) {
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
	if _, err := common.DynamoDB().PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(common.Tabela), Item: av}); err != nil {
		return Entrega{}, err
	}
	return toEntrega(it), nil
}

// removerEntrega apaga a entrega do grupo; errEntregaNaoExiste se não havia.
func removerEntrega(ctx context.Context, projetoID, atividadeID string) error {
	_, err := common.DynamoDB().DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(common.Tabela),
		Key: map[string]types.AttributeValue{
			"PK": str("PROJECT#" + projetoID), "SK": str("ENTREGA#" + atividadeID),
		},
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})
	if common.FalhouCondicao(err) {
		return errEntregaNaoExiste
	}
	return err
}

func entregasDoProjeto(ctx context.Context, projetoID string) ([]Entrega, error) {
	return paraEntregas(common.QueryNaParticao[itemEntrega](ctx, "PROJECT#"+projetoID, "ENTREGA#"))
}

func entregasDaAtividade(ctx context.Context, atividadeID string) ([]Entrega, error) {
	return paraEntregas(common.QueryNoIndice[itemEntrega](ctx, "ENTREGAS#"+atividadeID))
}

func paraEntregas(itens []itemEntrega, err error) ([]Entrega, error) {
	if err != nil {
		return nil, err
	}
	entregas := make([]Entrega, 0, len(itens))
	for _, it := range itens {
		entregas = append(entregas, toEntrega(it))
	}
	return entregas, nil
}
