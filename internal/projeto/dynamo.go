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
	gsiName   = os.Getenv("GSI_NAME") // ex: "GSI1" — ver módulo Terraform de DynamoDB
	ddb       *dynamodb.Client
)

func init() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(fmt.Sprintf("falha ao carregar config AWS: %v", err))
	}
	ddb = dynamodb.NewFromConfig(cfg)
}

// item modela o registro no DynamoDB. GSI1PK/GSI1SK existem só pra
// habilitar a Query "todos os projetos de um programa" sem Scan — mesmo
// princípio de acesso-primeiro que já usamos na allowlist de RGMs.
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

// associarOrientador faz UpdateItem só no campo orientadorId — não
// reescreve o item inteiro, evitando condição de corrida com outra
// escrita concorrente em nome/descricao/integrantes.
func associarOrientador(ctx context.Context, projetoID, orientadorID string) error {
	_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "PROJECT#" + projetoID},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		UpdateExpression: aws.String("SET orientadorId = :o"),
		// ConditionExpression garante que só atualiza se o projeto de fato
		// existir — sem isso, UpdateItem cria um item novo "vazio" com só
		// o orientadorId se o PK/SK não existirem, mascarando um
		// projetoId inválido como sucesso.
		ConditionExpression: aws.String("attribute_exists(PK)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":o": &types.AttributeValueMemberS{Value: orientadorID},
		},
	})
	return err
}
