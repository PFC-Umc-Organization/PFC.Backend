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

// listar faz Scan — aceitável aqui porque o volume de programas é baixo
// por natureza (dezenas: um por curso ofertado). Diferente da allowlist
// de alunos, que teria milhares de itens e por isso usa GetItem por
// chave, nunca Scan.
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

// existe confirma que um Programa com esse id foi de fato criado — usado
// pelo domínio projeto antes de aceitar um novo projeto vinculado a ele,
// pra não deixar criar projeto "órfão" apontando pra um programaId
// inventado.
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

// Existe expõe a checagem pro pacote projeto (mesmo módulo, pacotes
// irmãos — Go exige exportar mesmo dentro do mesmo módulo, só não
// dentro do mesmo pacote).
func Existe(ctx context.Context, id string) (bool, error) {
	return existe(ctx, id)
}
