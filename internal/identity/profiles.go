package identity

import (
	"context"
	"strings"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

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

func (p *DynamoProfiles) SaveStudent(ctx context.Context, rgm, name string) error {
	return dynamo.PutStruct(ctx, p.table, profileItem{PK: studentNamePrefix + rgm, SK: accountSK, Name: name}, dynamo.Write{})
}

func (p *DynamoProfiles) SaveStaff(ctx context.Context, sub, name string, role actor.Role) error {
	return dynamo.PutStruct(ctx, p.table, profileItem{PK: staffNamePrefix + sub, SK: accountSK, Name: name, Role: role}, dynamo.Write{})
}

func (p *DynamoProfiles) StudentNames(ctx context.Context, rgms []string) (map[string]string, error) {

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
