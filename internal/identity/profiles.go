package identity

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

const (
	studentNamePrefix = "ALUNO#"
	staffNamePrefix   = "EQUIPE#"
	accountSK         = "CONTA"
)

// Repositório
type DynamoProfiles struct {
	table *dynamo.Table
}

// Construção
func NewDynamoProfiles(t *dynamo.Table) *DynamoProfiles { return &DynamoProfiles{table: t} }

// Item
type profileItem struct {
	PK   string     `dynamodbav:"PK"`
	SK   string     `dynamodbav:"SK"`
	Name string     `dynamodbav:"nome"`
	Role actor.Role `dynamodbav:"perfil,omitempty"`
}

// Gravação
func (p *DynamoProfiles) SaveStudent(ctx context.Context, rgm, name string) error {
	_, err := p.table.UpdateItem(ctx, dynamo.Key(studentNamePrefix+rgm, accountSK), "SET nome = :n",
		dynamo.Write{Values: dynamo.Item{":n": dynamo.S(name)}})
	return err
}

// Gravação
func (p *DynamoProfiles) SaveStaff(ctx context.Context, sub, name string, role actor.Role) error {
	_, err := p.table.UpdateItem(ctx, dynamo.Key(staffNamePrefix+sub, accountSK), "SET nome = :n, perfil = :r",
		dynamo.Write{Values: dynamo.Item{":n": dynamo.S(name), ":r": dynamo.S(string(role))}})
	return err
}

// ---------------------------------------------------------------------------
// MFA
// ---------------------------------------------------------------------------

// Chave
func profileKey(u User) dynamo.Item {
	if u.Role == actor.RoleStudent {
		return dynamo.Key(studentNamePrefix+u.RGM, accountSK)
	}
	return dynamo.Key(staffNamePrefix+u.ID, accountSK)
}

// Gravação
func (p *DynamoProfiles) SaveMFA(ctx context.Context, u User, enabled bool) error {
	_, err := p.table.UpdateItem(ctx, profileKey(u), "SET mfaAtivo = :m, nome = if_not_exists(nome, :n)",
		dynamo.Write{Values: dynamo.Item{":m": &types.AttributeValueMemberBOOL{Value: enabled}, ":n": dynamo.S(u.Name)}})
	return err
}

// Consulta
func (p *DynamoProfiles) MFAEnabled(ctx context.Context, users []User) (map[string]bool, error) {
	keys := make([]dynamo.Item, 0, len(users))
	owner := make(map[string]string, len(users))
	for _, u := range users {
		if u.Role == actor.RoleStudent && u.RGM == "" {
			continue
		}
		k := profileKey(u)
		keys = append(keys, k)
		owner[dynamo.String(k, "PK")] = u.ID
	}
	items, err := p.table.BatchGet(ctx, keys, "PK", "mfaAtivo")
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, it := range items {
		if v, ok := it["mfaAtivo"].(*types.AttributeValueMemberBOOL); ok && v.Value {
			out[owner[dynamo.String(it, "PK")]] = true
		}
	}
	return out, nil
}

// Consulta
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

// Consulta
func (p *DynamoProfiles) Staff(ctx context.Context, sub string) (StaffProfile, bool, error) {
	it, found, err := dynamo.Get[profileItem](ctx, p.table, dynamo.Key(staffNamePrefix+sub, accountSK))
	return StaffProfile{Name: it.Name, Role: it.Role}, found, err
}
