package identity

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

// Items (single table):
//
//	student name  PK=ALUNO#<rgm>   SK=CONTA  nome
//	staff name    PK=EQUIPE#<sub>  SK=CONTA  nome, perfil
const (
	studentNamePrefix = "ALUNO#"
	staffNamePrefix   = "EQUIPE#"
	accountSK         = "CONTA"
)

type DynamoProfiles struct {
	table *dynamo.Table
}

func NewDynamoProfiles(t *dynamo.Table) *DynamoProfiles { return &DynamoProfiles{table: t} }

type profileItem struct {
	PK   string     `dynamodbav:"PK"`
	SK   string     `dynamodbav:"SK"`
	Name string     `dynamodbav:"nome"`
	Role actor.Role `dynamodbav:"perfil,omitempty"`
}

func (p *DynamoProfiles) put(ctx context.Context, it profileItem) error {
	av, err := attributevalue.MarshalMap(it)
	if err != nil {
		return err
	}
	_, err = p.table.Client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(p.table.Name), Item: av})
	return err
}

func (p *DynamoProfiles) SaveStudent(ctx context.Context, rgm, name string) error {
	return p.put(ctx, profileItem{PK: studentNamePrefix + rgm, SK: accountSK, Name: name})
}

func (p *DynamoProfiles) SaveStaff(ctx context.Context, sub, name string, role actor.Role) error {
	return p.put(ctx, profileItem{PK: staffNamePrefix + sub, SK: accountSK, Name: name, Role: role})
}

func (p *DynamoProfiles) StudentNames(ctx context.Context, rgms []string) (map[string]string, error) {
	// BatchGetItem refuses a request with a repeated key.
	seen := make(map[string]bool, len(rgms))
	keys := make([]dynamo.Item, 0, len(rgms))
	for _, rgm := range rgms {
		if !seen[rgm] {
			seen[rgm] = true
			keys = append(keys, dynamo.Key(studentNamePrefix+rgm, accountSK))
		}
	}
	items, err := p.table.BatchGet(ctx, keys, "PK", "nome")
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(items))
	for _, it := range items {
		names[strings.TrimPrefix(dynamo.String(it, "PK"), studentNamePrefix)] = dynamo.String(it, "nome")
	}
	return names, nil
}

func (p *DynamoProfiles) Staff(ctx context.Context, sub string) (StaffProfile, bool, error) {
	it, found, err := dynamo.Get[profileItem](ctx, p.table, dynamo.Key(staffNamePrefix+sub, accountSK))
	return StaffProfile{Name: it.Name, Role: it.Role}, found, err
}
