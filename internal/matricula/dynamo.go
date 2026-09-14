package matricula

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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


const batchSize = 25


func gravarRGMs(ctx context.Context, rgms []string) []RGMFalha {
	var falhas []RGMFalha

	for i := 0; i < len(rgms); i += batchSize {
		chunk := rgms[i:min(i+batchSize, len(rgms))]

		writes := make([]types.WriteRequest, 0, len(chunk))
		for _, rgm := range chunk {
			writes = append(writes, types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: map[string]types.AttributeValue{
						"PK":     &types.AttributeValueMemberS{Value: "STUDENT#" + rgm},
						"SK":     &types.AttributeValueMemberS{Value: "PROFILE"},
						"status": &types.AttributeValueMemberS{Value: "ACTIVE"},
					},
				},
			})
		}

		if _, err := ddb.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{tableName: writes},
		}); err != nil {
			for _, rgm := range chunk {
				falhas = append(falhas, RGMFalha{RGM: rgm, Erro: err.Error()})
			}
		}
	}

	return falhas
}


func listarRGMs(ctx context.Context) ([]Matricula, error) {
	out, err := ddb.Scan(ctx, &dynamodb.ScanInput{
		TableName:        aws.String(tableName),
		FilterExpression: aws.String("begins_with(PK, :prefixo) AND SK = :perfil"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":prefixo": &types.AttributeValueMemberS{Value: "STUDENT#"},
			":perfil":  &types.AttributeValueMemberS{Value: "PROFILE"},
		},
	})
	if err != nil {
		return nil, err
	}

	matriculas := make([]Matricula, 0, len(out.Items))
	for _, i := range out.Items {
		pk, ok := i["PK"].(*types.AttributeValueMemberS)
		if !ok {
			continue
		}

		status := "INATIVO"
		if s, ok := i["status"].(*types.AttributeValueMemberS); ok && s.Value == "ACTIVE" {
			status = "ATIVO"
		}

		matriculas = append(matriculas, Matricula{
			RGM:    pk.Value[len("STUDENT#"):],
			Status: status,
		})
	}
	return matriculas, nil
}


func existeRGM(ctx context.Context, rgm string) (bool, error) {
	out, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "STUDENT#" + rgm},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
	})
	if err != nil {
		return false, err
	}
	return out.Item != nil, nil
}


func Existe(ctx context.Context, rgm string) (bool, error) {
	return existeRGM(ctx, rgm)
}


func removerRGMs(ctx context.Context, rgms []string) []RGMFalha {
	var falhas []RGMFalha

	for i := 0; i < len(rgms); i += batchSize {
		chunk := rgms[i:min(i+batchSize, len(rgms))]

		writes := make([]types.WriteRequest, 0, len(chunk))
		for _, rgm := range chunk {
			writes = append(writes, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: "STUDENT#" + rgm},
						"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
					},
				},
			})
		}

		if _, err := ddb.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{tableName: writes},
		}); err != nil {
			for _, rgm := range chunk {
				falhas = append(falhas, RGMFalha{RGM: rgm, Erro: err.Error()})
			}
		}
	}

	return falhas
}
