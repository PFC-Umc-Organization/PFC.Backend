// Package app is the composition root: it builds the shared clients and
// wires every module into the router.
package app

import (
	"context"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/academic"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/deliveries"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/identity"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/materials"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/config"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/storage"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/references"
)

type Deps struct {
	Config  config.Config
	Table   *dynamo.Table
	Cognito *cognitoidentityprovider.Client
	S3      *s3.Client
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
		S3:      s3.NewFromConfig(aws, func(o *s3.Options) { o.UsePathStyle = cfg.S3PathStyle }),
	}, nil
}

func New(d Deps) *httpapi.Router {
	groupsStore := groups.NewDynamoStore(d.Table)
	academicSvc := academic.NewService(academic.NewDynamoStore(d.Table), groupsStore)
	identitySvc := identity.NewService(identity.NewCognito(d.Cognito, d.Config.CognitoClientID, d.Config.CognitoUserPoolID), academicSvc, identity.NewDynamoProfiles(d.Table))
	groupsSvc := groups.NewService(groupsStore, academicForGroups{academicSvc}, identitySvc)
	identitySvc.UseAdvising(groupsSvc)
	catalog := references.NewCatalog(references.CatalogConfig{
		CrossrefURL:    d.Config.CrossrefURL,
		OpenAlexURL:    d.Config.OpenAlexURL,
		ContactEmail:   d.Config.ContactEmail,
		OpenAlexAPIKey: d.Config.OpenAlexAPIKey,
	})

	// Without a bucket the interfaces stay nil (not a nil *storage.Bucket):
	// file fields keep only names and materials are links only.
	var deliveryFiles deliveries.Files
	var materialFiles materials.Files
	if d.Config.UploadsBucket != "" {
		b := storage.New(d.S3, d.Config.UploadsBucket)
		deliveryFiles, materialFiles = b, b
	}

	r := httpapi.NewRouter()
	identity.Routes(r, identitySvc)
	academic.Routes(r, academicSvc)
	groups.Routes(r, groupsSvc)
	deliveriesSvc := deliveries.NewService(deliveries.NewDynamoStore(d.Table), groupsSvc, deliveryFiles)
	groupsSvc.OnProjectDeleted(deliveriesSvc.ProjectDeleted)
	deliveries.Routes(r, deliveriesSvc)
	materials.Routes(r, materials.NewService(materials.NewDynamoStore(d.Table), academicSvc, materialFiles))
	references.Routes(r, references.NewService(references.NewDynamoStore(d.Table), catalog, groupsSvc))
	return r
}

// academicForGroups hands groups the cohort in its own type, so neither
// module imports the other.
type academicForGroups struct{ *academic.Service }

func (a academicForGroups) ProgramIDs(ctx context.Context) ([]string, error) {
	programs, err := a.Service.ListPrograms(ctx)
	ids := make([]string, 0, len(programs))
	for _, p := range programs {
		ids = append(ids, p.ID)
	}
	return ids, err
}

func (a academicForGroups) Cohort(ctx context.Context, id string) (groups.Cohort, bool, error) {
	c, found, err := a.Service.Cohort(ctx, id)
	return groups.Cohort{ID: c.ID, Course: c.Course, Shift: c.Shift, Term: c.Term}, found, err
}
