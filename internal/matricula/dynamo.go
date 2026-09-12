package matricula

import (
	"context"
	"fmt"
	"os"

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

// batchSize é o limite da API BatchWriteItem — não é escolha arbitrária,
// é teto rígido da AWS.
const batchSize = 25

// gravarRGMs grava cada RGM como STUDENT#<rgm>/PROFILE com status ACTIVE.
// Falhas de um chunk não interrompem os demais — cada chunk é reportado
// separadamente em caso de erro, o resto do lote segue.
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

// removerRGMs remove os itens da allowlist — não afeta nenhuma conta que
// já exista no Cognito, só o registro de matrícula.
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
