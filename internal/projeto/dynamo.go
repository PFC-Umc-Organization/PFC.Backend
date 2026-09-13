package projeto

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

var (
	tableName = os.Getenv("TABLE_NAME")
	gsiName   = os.Getenv("GSI_NAME")
	ddb       *dynamodb.Client
)

func init() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(fmt.Sprintf("falha ao carregar config AWS: %v", err))
	}
	ddb = dynamodb.NewFromConfig(cfg)
}

type item struct {
	PK           string   `dynamodbav:"PK"`
	SK           string   `dynamodbav:"SK"`
	GSI1PK       string   `dynamodbav:"GSI1PK"`
	GSI1SK       string   `dynamodbav:"GSI1SK"`
	Nome         string   `dynamodbav:"nome"`
	Descricao    string   `dynamodbav:"descricao"`
	ProgramaID   string   `dynamodbav:"programaId"`
	Integrantes  []string `dynamodbav:"integrantes"`
	OrientadorID string   `dynamodbav:"orientadorId"`
}

func toProjeto(it item) Projeto {
	return Projeto{
		ID:           it.PK[len("PROJECT#"):],
		Nome:         it.Nome,
		Descricao:    it.Descricao,
		ProgramaID:   it.ProgramaID,
		Integrantes:  it.Integrantes,
		OrientadorID: it.OrientadorID,
	}
}

func salvar(ctx context.Context, programaID string, novo NovoProjeto) (Projeto, error) {
	id := uuid.NewString()

	it := item{
		PK:          "PROJECT#" + id,
		SK:          "PROFILE",
		GSI1PK:      "PROGRAM#" + programaID,
		GSI1SK:      "PROJECT#" + id,
		Nome:        novo.Nome,
		Descricao:   novo.Descricao,
		ProgramaID:  programaID,
		Integrantes: novo.Integrantes,
	}

	av, err := attributevalue.MarshalMap(it)
	if err != nil {
		return Projeto{}, fmt.Errorf("falha ao serializar projeto: %w", err)
	}

	if _, err := ddb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      av,
	}); err != nil {
		return Projeto{}, err
	}

	return toProjeto(it), nil
}

func listarPorPrograma(ctx context.Context, programaID string) ([]Projeto, error) {
	out, err := ddb.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String(gsiName),
		KeyConditionExpression: aws.String("GSI1PK = :programa"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":programa": &types.AttributeValueMemberS{Value: "PROGRAM#" + programaID},
		},
	})
	if err != nil {
		return nil, err
	}

	projetos := make([]Projeto, 0, len(out.Items))
	for _, i := range out.Items {
		var it item
		if err := attributevalue.UnmarshalMap(i, &it); err != nil {
			continue
		}
		projetos = append(projetos, toProjeto(it))
	}
	return projetos, nil
}

func associarOrientador(ctx context.Context, projetoID, orientadorID string) error {
	_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PROJECT#" + projetoID},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		UpdateExpression:    aws.String("SET orientadorId = :o"),
		ConditionExpression: aws.String("attribute_exists(PK)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":o": &types.AttributeValueMemberS{Value: orientadorID},
		},
	})
	return err
}

// adicionarIntegrante insere um RGM na lista de integrantes usando
// list_append, sem reescrever o item inteiro.
func adicionarIntegrante(ctx context.Context, projetoID, rgm string) error {
	_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PROJECT#" + projetoID},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		UpdateExpression:    aws.String("SET integrantes = list_append(if_not_exists(integrantes, :vazio), :novo)"),
		ConditionExpression: aws.String("attribute_exists(PK)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":vazio": &types.AttributeValueMemberL{Value: []types.AttributeValue{}},
			":novo": &types.AttributeValueMemberL{Value: []types.AttributeValue{
				&types.AttributeValueMemberS{Value: rgm},
			}},
		},
	})
	return err
}

// removerIntegrante lê o projeto, filtra o RGM da lista em memória e
// regrava tudo — DynamoDB não remove valor de lista direto, só por índice.
func removerIntegrante(ctx context.Context, projetoID, rgm string) error {
	out, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PROJECT#" + projetoID},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
	})
	if err != nil {
		return err
	}
	if out.Item == nil {
		return fmt.Errorf("projeto não encontrado")
	}

	var it item
	if err := attributevalue.UnmarshalMap(out.Item, &it); err != nil {
		return err
	}

	restantes := make([]string, 0, len(it.Integrantes))
	for _, r := range it.Integrantes {
		if r != rgm {
			restantes = append(restantes, r)
		}
	}

	novosValores := make([]types.AttributeValue, 0, len(restantes))
	for _, r := range restantes {
		novosValores = append(novosValores, &types.AttributeValueMemberS{Value: r})
	}

	_, err = ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PROJECT#" + projetoID},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		UpdateExpression: aws.String("SET integrantes = :lista"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":lista": &types.AttributeValueMemberL{Value: novosValores},
		},
	})
	return err
}

func atualizarProjeto(ctx context.Context, projetoID string, dados AtualizarProjeto) error {
	_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PROJECT#" + projetoID},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		UpdateExpression:    aws.String("SET nome = :n, descricao = :d"),
		ConditionExpression: aws.String("attribute_exists(PK)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":n": &types.AttributeValueMemberS{Value: dados.Nome},
			":d": &types.AttributeValueMemberS{Value: dados.Descricao},
		},
	})
	return err
}

func deletarProjeto(ctx context.Context, projetoID string) error {
	_, err := ddb.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PROJECT#" + projetoID},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})
	return err
}