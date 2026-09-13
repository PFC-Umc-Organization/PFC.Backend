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

// listarRGMs faz Scan na allowlist. Aceitável aqui pelo mesmo motivo que em
// `programa`: volume baixo (uma turma inteira tem no máximo algumas centenas
// de RGMs pré-autorizados, não milhões).
//
// O valor gravado em `status` é "ACTIVE" (ver gravarRGMs), mas o frontend
// espera "ATIVO"/"INATIVO" (mesmo union de StatusUsuario) — a tradução é
// feita aqui, não no armazenamento, pra não precisar migrar os itens já
// gravados.
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

// existeRGM confirma que um RGM está pré-autorizado — usado pelo pacote
// projeto antes de aceitar um integrante novo, pra não deixar adicionar um
// RGM que não está na allowlist.
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

// Existe expõe a checagem pro pacote projeto (mesmo módulo, pacotes irmãos).
func Existe(ctx context.Context, rgm string) (bool, error) {
	return existeRGM(ctx, rgm)
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
