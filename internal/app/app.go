// Package app is the composition root: it builds the shared clients and
// wires every module into the router.
package app

import (
	"context"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/academic"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/deliveries"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/identity"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/config"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/references"
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
	groupsStore := groups.NewDynamoStore(d.Table)
	academicSvc := academic.NewService(academic.NewDynamoStore(d.Table), groupsStore)
	identitySvc := identity.NewService(identity.NewCognito(d.Cognito, d.Config.CognitoClientID, d.Config.CognitoUserPoolID), academicSvc)
	groupsSvc := groups.NewService(groupsStore, academicSvc, identitySvc)
	catalog := references.NewCatalog(references.CatalogConfig{
		CrossrefURL:    d.Config.CrossrefURL,
		OpenAlexURL:    d.Config.OpenAlexURL,
		ContactEmail:   d.Config.ContactEmail,
		OpenAlexAPIKey: d.Config.OpenAlexAPIKey,
	})

	r := httpapi.NewRouter()
	identity.Routes(r, identitySvc)
	academic.Routes(r, academicSvc)
	groups.Routes(r, groupsSvc)
	deliveries.Routes(r, deliveries.NewService(deliveries.NewDynamoStore(d.Table), groupsSvc))
	references.Routes(r, references.NewService(references.NewDynamoStore(d.Table), catalog, groupsSvc))
	return r
}
