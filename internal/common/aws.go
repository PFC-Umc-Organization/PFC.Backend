package common

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

// Transitional bridge to the platform packages for the code not yet moved
// into modules. Removed in phase 7 of the refactor.

var (
	Tabela = os.Getenv("TABLE_NAME")
	Indice = os.Getenv("GSI_NAME")
)

var (
	mu        sync.Mutex
	ddbClient *dynamodb.Client
	cipClient *cognitoidentityprovider.Client
)

// Use makes the legacy packages share the clients built by the composition
// root.
func Use(ddb *dynamodb.Client, cip *cognitoidentityprovider.Client) {
	mu.Lock()
	defer mu.Unlock()
	ddbClient, cipClient = ddb, cip
}

func DynamoDB() *dynamodb.Client {
	mu.Lock()
	defer mu.Unlock()
	if ddbClient == nil {
		ddbClient = dynamodb.NewFromConfig(loadAWS())
	}
	return ddbClient
}

func Cognito() *cognitoidentityprovider.Client {
	mu.Lock()
	defer mu.Unlock()
	if cipClient == nil {
		cipClient = cognitoidentityprovider.NewFromConfig(loadAWS())
	}
	return cipClient
}

func loadAWS() (cfg aws.Config) {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(fmt.Sprintf("load AWS config: %v", err))
	}
	return cfg
}

func table() *dynamo.Table {
	return &dynamo.Table{Client: DynamoDB(), Name: Tabela, Index: Indice}
}
