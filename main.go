package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var (
	tableName     = os.Getenv("TABLE_NAME")
	allowedDomain = os.Getenv("ALLOWED_DOMAIN")
	ddbClient     *dynamodb.Client
)

func init() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(fmt.Sprintf("falha ao carregar config AWS: %v", err))
	}
	ddbClient = dynamodb.NewFromConfig(cfg)
}

func extractRGM(email string) (string, error) {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[1], allowedDomain) {
		return "", fmt.Errorf("domínio não autorizado: %s", email)
	}
	return parts[0], nil
}

func isEnrolled(ctx context.Context, rgm string) error {
	out, err := ddbClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "STUDENT#" + rgm},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		},
	})
	if err != nil {
		return fmt.Errorf("falha ao consultar allowlist")
	}
	if out.Item == nil {
		return fmt.Errorf("RGM não matriculado: %s", rgm)
	}

	statusAttr, ok := out.Item["status"].(*types.AttributeValueMemberS)
	if !ok || statusAttr.Value != "ACTIVE" {
		return fmt.Errorf("matrícula inativa para RGM: %s", rgm)
	}
	return nil
}

func handler(ctx context.Context, event events.CognitoEventUserPoolsPreSignup) (events.CognitoEventUserPoolsPreSignup, error) {
	// AdminCreateUser só é chamável por quem já passou pela barreira de IAM
	// (grupo admin + Identity Pool role, ou você mesmo no console). A validação
	// de domínio/allowlist abaixo só faz sentido pro self sign-up via Hosted UI.
	if event.TriggerSource == "PreSignUp_AdminCreateUser" {
		return event, nil
	}

	email, ok := event.Request.UserAttributes["email"]
	if !ok || email == "" {
		return event, fmt.Errorf("email attribute ausente")
	}

	rgm, err := extractRGM(email)
	if err != nil {
		return event, err
	}

	if err := isEnrolled(ctx, rgm); err != nil {
		return event, err
	}

	return event, nil
}

func main() {
	lambda.Start(handler)
}