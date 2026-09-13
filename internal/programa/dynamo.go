package programa

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
	gsiName   = os.Getenv("GSI_NAME") // mesmo índice usado pelo pacote projeto
	ddb       *dynamodb.Client
)

var errProgramaComProjetos = fmt.Errorf("programa possui projetos vinculados")

func init() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(fmt.Sprintf("falha ao carregar config AWS: %v", err))
	}
	ddb = dynamodb.NewFromConfig(cfg)
}

type item struct {
	PK      string `dynamodbav:"PK"`
	SK      string `dynamodbav:"SK"`
	CursoID string `dynamodbav:"cursoId"`
}

func salvar(ctx context.Context, novo NovoPrograma) (Programa, error) {
	id := uuid.NewString()

	it := item{
		PK:      "PROGRAM#" + id,
		SK:      "PROFILE",
		CursoID: novo.CursoID,
	}

	av, err := attributevalue.MarshalMap(it)
	if err != nil {
		return Programa{}, fmt.Errorf("falha ao serializar programa: %w", err)
	}

	if _, err := ddb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      av,
	}); err != nil {
		return Programa{}, err
	}

	return Programa{ID: id, CursoID: novo.CursoID}, nil
}

func listar(ctx context.Context) ([]Programa, error) {
	out, err := ddb.Scan(ctx, &dynamodb.ScanInput{
		TableName:        aws.String(tableName),
		FilterExpression: aws.String("begins_with(PK, :prefixo) AND SK = :perfil"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefixo": &types.AttributeValueMemberS{Value: "PROGRAM#"},
			":perfil":  &types.AttributeValueMemberS{Value: "PROFILE"},
		},
	})
	if err != nil {
		return nil, err
	}

	programas := make([]Programa, 0, len(out.Items))
	for _, i := range out.Items {
		var it item
		if err := attributevalue.UnmarshalMap(i, &it); err != nil {
			continue
		}
		id := it.PK[len("PROGRAM#"):]
		programas = append(programas, Programa{ID: id, CursoID: it.CursoID})
	}
	return programas, nil
}

func existe(ctx context.Context, id string) (bool, error) {
	out, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PROGRAM#" + id},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
	})
	if err != nil {
		return false, err
	}
	return out.Item != nil, nil
}

func Existe(ctx context.Context, id string) (bool, error) {
	return existe(ctx, id)
}

// atualizar troca o cursoId de um programa já existente, com
// ConditionExpression pra não criar um item "vazio" caso o id não exista.
func atualizar(ctx context.Context, id string, dados AtualizarPrograma) error {
	_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PROGRAM#" + id},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		UpdateExpression:    aws.String("SET cursoId = :c"),
		ConditionExpression: aws.String("attribute_exists(PK)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":c": &types.AttributeValueMemberS{Value: dados.CursoID},
		},
	})
	return err
}

// possuiProjetos consulta o GSI1 pra ver se algum projeto ainda referencia
// esse programa — evita importar o pacote projeto aqui (import cíclico).
func possuiProjetos(ctx context.Context, id string) (bool, error) {
	out, err := ddb.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String(gsiName),
		KeyConditionExpression: aws.String("GSI1PK = :programa"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":programa": &types.AttributeValueMemberS{Value: "PROGRAM#" + id},
		},
		Limit: aws.Int32(1),
	})
	if err != nil {
		return false, err
	}
	return len(out.Items) > 0, nil
}

// deletar recusa remover o programa se ainda existir projeto vinculado.
func deletar(ctx context.Context, id string) error {
	temProjetos, err := possuiProjetos(ctx, id)
	if err != nil {
		return err
	}
	if temProjetos {
		return errProgramaComProjetos
	}

	_, err = ddb.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PROGRAM#" + id},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})
	return err
}