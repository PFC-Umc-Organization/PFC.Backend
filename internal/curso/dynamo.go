package curso

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

var errCursoComPrograma = fmt.Errorf("curso possui programa vinculado")

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
	Nome    string `dynamodbav:"nome"`
	Turno   string `dynamodbav:"turno"`
	Periodo string `dynamodbav:"periodo"`
}

func toCurso(it item) Curso {
	return Curso{
		ID:      it.PK[len("CURSO#"):],
		Nome:    it.Nome,
		Turno:   it.Turno,
		Periodo: it.Periodo,
	}
}

func salvar(ctx context.Context, novo NovoCurso) (Curso, error) {
	id := uuid.NewString()

	it := item{
		PK:      "CURSO#" + id,
		SK:      "PROFILE",
		Nome:    novo.Nome,
		Turno:   novo.Turno,
		Periodo: novo.Periodo,
	}

	av, err := attributevalue.MarshalMap(it)
	if err != nil {
		return Curso{}, fmt.Errorf("falha ao serializar turma: %w", err)
	}

	if _, err := ddb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      av,
	}); err != nil {
		return Curso{}, err
	}

	return toCurso(it), nil
}

// listar faz Scan — aceitável aqui pelo mesmo motivo que em `programa`:
// volume baixo (algumas turmas por período, não milhares).
func listar(ctx context.Context) ([]Curso, error) {
	out, err := ddb.Scan(ctx, &dynamodb.ScanInput{
		TableName:        aws.String(tableName),
		FilterExpression: aws.String("begins_with(PK, :prefixo) AND SK = :perfil"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefixo": &types.AttributeValueMemberS{Value: "CURSO#"},
			":perfil":  &types.AttributeValueMemberS{Value: "PROFILE"},
		},
	})
	if err != nil {
		return nil, err
	}

	cursos := make([]Curso, 0, len(out.Items))
	for _, i := range out.Items {
		var it item
		if err := attributevalue.UnmarshalMap(i, &it); err != nil {
			continue
		}
		cursos = append(cursos, toCurso(it))
	}
	return cursos, nil
}

func atualizar(ctx context.Context, id string, dados AtualizarCurso) error {
	_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "CURSO#" + id},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		UpdateExpression:    aws.String("SET nome = :n, turno = :t, periodo = :p"),
		ConditionExpression: aws.String("attribute_exists(PK)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":n": &types.AttributeValueMemberS{Value: dados.Nome},
			":t": &types.AttributeValueMemberS{Value: dados.Turno},
			":p": &types.AttributeValueMemberS{Value: dados.Periodo},
		},
	})
	return err
}

// possuiPrograma verifica (via Scan, mesmo motivo de volume baixo) se
// algum Programa já referencia este curso — evita apagar uma turma que
// ainda está em uso, mesma regra que `programa.deletar` já aplica em
// relação a projetos vinculados.
func possuiPrograma(ctx context.Context, cursoID string) (bool, error) {
	out, err := ddb.Scan(ctx, &dynamodb.ScanInput{
		TableName:        aws.String(tableName),
		FilterExpression: aws.String("begins_with(PK, :prefixo) AND cursoId = :c"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefixo": &types.AttributeValueMemberS{Value: "PROGRAM#"},
			":c":       &types.AttributeValueMemberS{Value: cursoID},
		},
	})
	if err != nil {
		return false, err
	}
	return len(out.Items) > 0, nil
}

func deletar(ctx context.Context, id string) error {
	temPrograma, err := possuiPrograma(ctx, id)
	if err != nil {
		return err
	}
	if temPrograma {
		return errCursoComPrograma
	}

	_, err = ddb.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "CURSO#" + id},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})
	return err
}
