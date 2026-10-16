// Package app is the composition root: it builds the shared clients and
// wires every module into the router.
package app

import (
	"context"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/config"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

type Deps struct {
	Config  config.Config
	Table   *dynamo.Table
	Cognito *cognitoidentityprovider.Client
}

func Load(ctx context.Context) (Deps, error) {
	cfg, err := config.FromEnv()
	if err != nil {
		return Deps{}, err
	}
	aws, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return Deps{}, err
	}
	return Deps{
		Config:  cfg,
		Table:   &dynamo.Table{Client: dynamodb.NewFromConfig(aws), Name: cfg.TableName, Index: cfg.IndexName},
		Cognito: cognitoidentityprovider.NewFromConfig(aws),
	}, nil
}

func New(d Deps) *httpapi.Router {
	common.Use(d.Table.Client, d.Cognito)
	r := httpapi.NewRouter()
	routes(r)
	return r
}
