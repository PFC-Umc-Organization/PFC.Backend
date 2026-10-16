package groups

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

// Items (single table):
//
//	project      PK=PROJECT#<id>  SK=PROFILE       GSI1PK=PROGRAM#<pid>  GSI1SK=PROJECT#<id>  integrantes, orientadorId, ...
//	membership   PK=MEMBRO#<rgm>  SK=PROJETO       projetoId
//	invitation   PK=PROJECT#<id>  SK=CONVITE#<rgm> GSI1PK=CONVITE#<rgm>  GSI1SK=PROJECT#<id>
//
// The membership item is written with attribute_not_exists in the same
// transaction that changes `integrantes`, which is what keeps a student in a
// single project under concurrent requests. Every write to `integrantes`
// moves memberships along.
const (
	projectPrefix    = "PROJECT#"
	programPrefix    = "PROGRAM#"
	memberPrefix     = "MEMBRO#"
	invitationPrefix = "CONVITE#"
	profileSK        = "PROFILE"
	membershipSK     = "PROJETO"

	absent  = "attribute_not_exists(PK)"
	present = "attribute_exists(PK)"
)

type DynamoStore struct {
	table *dynamo.Table
}

func NewDynamoStore(t *dynamo.Table) *DynamoStore { return &DynamoStore{table: t} }

type projectItem struct {
	PK          string   `dynamodbav:"PK"`
	SK          string   `dynamodbav:"SK"`
	GSI1PK      string   `dynamodbav:"GSI1PK"`
	GSI1SK      string   `dynamodbav:"GSI1SK"`
	Name        string   `dynamodbav:"nome"`
	Description string   `dynamodbav:"descricao"`
	ProgramID   string   `dynamodbav:"programaId"`
	Members     []string `dynamodbav:"integrantes"`
	AdvisorID   string   `dynamodbav:"orientadorId"`
}

// project never returns nil Members: the frontend calls
// integrantes.includes and breaks on null.
func (it projectItem) project() Project {
	if it.Members == nil {
		it.Members = []string{}
	}
	return Project{
		ID:          strings.TrimPrefix(it.PK, projectPrefix),
		Name:        it.Name,
		Description: it.Description,
		ProgramID:   it.ProgramID,
		Members:     it.Members,
		AdvisorID:   it.AdvisorID,
	}
}

type invitationItem struct {
	PK          string `dynamodbav:"PK"`
	SK          string `dynamodbav:"SK"`
	GSI1PK      string `dynamodbav:"GSI1PK"`
	GSI1SK      string `dynamodbav:"GSI1SK"`
	ProjectName string `dynamodbav:"projetoNome"`
	InvitedBy   string `dynamodbav:"convidadoPor"`
	CreatedAt   string `dynamodbav:"criadoEm"`
}

func (it invitationItem) invitation() Invitation {
	return Invitation{
		ProjectID:   strings.TrimPrefix(it.PK, projectPrefix),
		ProjectName: it.ProjectName,
		RGM:         strings.TrimPrefix(it.SK, invitationPrefix),
		InvitedBy:   it.InvitedBy,
		CreatedAt:   it.CreatedAt,
	}
}

func projectKey(id string) dynamo.Item     { return dynamo.Key(projectPrefix+id, profileSK) }
func membershipKey(rgm string) dynamo.Item { return dynamo.Key(memberPrefix+rgm, membershipSK) }
func invitationKey(projectID, rgm string) dynamo.Item {
	return dynamo.Key(projectPrefix+projectID, invitationPrefix+rgm)
}

func (s *DynamoStore) join(rgm, projectID string) types.TransactWriteItem {
	m := membershipKey(rgm)
	m["projetoId"] = dynamo.S(projectID)
	return s.table.Put(m, absent, nil)
}

// leave only deletes a membership that points to this project (or none).
func (s *DynamoStore) leave(rgm, projectID string) types.TransactWriteItem {
	return s.table.Delete(membershipKey(rgm), "attribute_not_exists(PK) OR projetoId = :project",
		dynamo.Item{":project": dynamo.S(projectID)})
}

func rgmList(rgms []string) *types.AttributeValueMemberL {
	values := make([]types.AttributeValue, 0, len(rgms))
	for _, r := range rgms {
		values = append(values, dynamo.S(r))
	}
	return &types.AttributeValueMemberL{Value: values}
}

func (s *DynamoStore) CreateProject(ctx context.Context, p Project) error {
	item, err := attributevalue.MarshalMap(projectItem{
		PK: projectPrefix + p.ID, SK: profileSK, GSI1PK: programPrefix + p.ProgramID, GSI1SK: projectPrefix + p.ID,
		Name: p.Name, Description: p.Description, ProgramID: p.ProgramID, Members: p.Members,
	})
	if err != nil {
		return err
	}
	writes := []types.TransactWriteItem{s.table.Put(item, absent, nil)}
	for _, rgm := range p.Members {
		writes = append(writes, s.join(rgm, p.ID))
	}
	err = s.table.Transact(ctx, writes...)
	var cond *dynamo.ConditionError
	if errors.As(err, &cond) {
		return errMemberTaken
	}
	return err
}

func (s *DynamoStore) GetProject(ctx context.Context, id string) (Project, bool, error) {
	it, found, err := dynamo.Get[projectItem](ctx, s.table, projectKey(id))
	if err != nil || !found {
		return Project{}, found, err
	}
	return it.project(), true, nil
}

func (s *DynamoStore) ProjectsOfProgram(ctx context.Context, programID string) ([]Project, error) {
	items, err := dynamo.QueryIndex[projectItem](ctx, s.table, programPrefix+programID)
	if err != nil {
		return nil, err
	}
	projects := make([]Project, 0, len(items))
	for _, it := range items {
		projects = append(projects, it.project())
	}
	return projects, nil
}

func (s *DynamoStore) ProgramHasProjects(ctx context.Context, programID string) (bool, error) {
	out, err := s.table.Client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.table.Name),
		IndexName:                 aws.String(s.table.Index),
		KeyConditionExpression:    aws.String("GSI1PK = :program"),
		ExpressionAttributeValues: dynamo.Item{":program": dynamo.S(programPrefix + programID)},
		ProjectionExpression:      aws.String("PK"),
		Limit:                     aws.Int32(1),
	})
	if err != nil {
		return false, err
	}
	return len(out.Items) > 0, nil
}

func (s *DynamoStore) ProjectOfMember(ctx context.Context, rgm string) (Project, bool, error) {
	m, found, err := dynamo.Get[struct {
		ProjectID string `dynamodbav:"projetoId"`
	}](ctx, s.table, membershipKey(rgm), "projetoId")
	if err != nil || !found {
		return Project{}, false, err
	}
	return s.GetProject(ctx, m.ProjectID)
}

// ProjectsOfMembers maps each RGM that already has a project to its id.
func (s *DynamoStore) ProjectsOfMembers(ctx context.Context, rgms []string) (map[string]string, error) {
	keys := make([]dynamo.Item, 0, len(rgms))
	for _, rgm := range rgms {
		keys = append(keys, membershipKey(rgm))
	}
	items, err := s.table.BatchGet(ctx, keys, "PK", "projetoId")
	if err != nil {
		return nil, err
	}
	taken := make(map[string]string, len(items))
	for _, it := range items {
		taken[strings.TrimPrefix(dynamo.String(it, "PK"), memberPrefix)] = dynamo.String(it, "projetoId")
	}
	return taken, nil
}

func (s *DynamoStore) update(ctx context.Context, id, expression string, values dynamo.Item) error {
	in := &dynamodb.UpdateItemInput{
		TableName:           aws.String(s.table.Name),
		Key:                 projectKey(id),
		UpdateExpression:    aws.String(expression),
		ConditionExpression: aws.String(present),
	}
	if len(values) > 0 {
		in.ExpressionAttributeValues = values
	}
	_, err := s.table.Client.UpdateItem(ctx, in)
	if dynamo.ConditionFailed(err) {
		return errNotFound
	}
	return err
}

func (s *DynamoStore) UpdateDetails(ctx context.Context, id, name, description string) error {
	return s.update(ctx, id, "SET nome = :n, descricao = :d", dynamo.Item{":n": dynamo.S(name), ":d": dynamo.S(description)})
}

func (s *DynamoStore) SetAdvisor(ctx context.Context, id, advisorID string) error {
	return s.update(ctx, id, "SET orientadorId = :o", dynamo.Item{":o": dynamo.S(advisorID)})
}

func (s *DynamoStore) ClearAdvisor(ctx context.Context, id string) error {
	return s.update(ctx, id, "REMOVE orientadorId", nil)
}

// AddMember appends the RGM and creates its membership atomically. current
// is the list the caller read: when empty the list is written whole, since
// old projects may hold no list or NULL and list_append fails on NULL. The
// condition rejects a repeated RGM and a full group (errStale); the
// membership rejects a student who already has a project (errMemberTaken).
func (s *DynamoStore) AddMember(ctx context.Context, id, rgm string, current []string) error {
	added := rgmList([]string{rgm})
	var update types.TransactWriteItem
	if len(current) == 0 {
		update = s.table.Update(projectKey(id), "SET integrantes = :new",
			"attribute_exists(PK) AND (attribute_not_exists(integrantes) OR attribute_type(integrantes, :null) OR size(integrantes) = :zero)",
			dynamo.Item{":new": added, ":null": dynamo.S("NULL"), ":zero": dynamo.N(0)})
	} else {
		update = s.table.Update(projectKey(id), "SET integrantes = list_append(integrantes, :new)",
			"attribute_exists(PK) AND NOT contains(integrantes, :rgm) AND size(integrantes) < :max",
			dynamo.Item{":new": added, ":rgm": dynamo.S(rgm), ":max": dynamo.N(MaxMembers)})
	}
	err := s.table.Transact(ctx, update, s.join(rgm, id))
	switch {
	case dynamo.FailedAt(err, 1):
		return errMemberTaken
	case dynamo.FailedAt(err, 0):
		return errStale
	}
	return err
}

// RemoveMember conditions on the list it read: otherwise a student joining
// in between would be dropped, and on a deleted project the update would
// recreate a bare item.
func (s *DynamoStore) RemoveMember(ctx context.Context, id, rgm string) error {
	it, found, err := dynamo.Get[projectItem](ctx, s.table, projectKey(id))
	if err != nil {
		return err
	}
	if !found {
		return errNotFound
	}
	remaining := make([]string, 0, len(it.Members))
	for _, r := range it.Members {
		if r != rgm {
			remaining = append(remaining, r)
		}
	}
	if len(remaining) == len(it.Members) {
		return errNotMember
	}
	err = s.table.Transact(ctx,
		s.table.Update(projectKey(id), "SET integrantes = :list", "attribute_exists(PK) AND integrantes = :previous",
			dynamo.Item{":list": rgmList(remaining), ":previous": rgmList(it.Members)}),
		s.leave(rgm, id),
	)
	var cond *dynamo.ConditionError
	if errors.As(err, &cond) {
		return errStale
	}
	return err
}

// DeleteProject removes the project, its memberships and everything in its
// partition (invitations, submissions, references): left behind, students
// saw invitations to missing groups and submissions stayed in the
// activity's index.
func (s *DynamoStore) DeleteProject(ctx context.Context, id string) error {
	it, found, err := dynamo.Get[projectItem](ctx, s.table, projectKey(id), "PK", "integrantes")
	if err != nil {
		return err
	}
	if !found {
		return errNotFound
	}
	first := s.table.Delete(projectKey(id), "attribute_exists(PK) AND integrantes = :list", dynamo.Item{":list": rgmList(it.Members)})
	if it.Members == nil {
		first = s.table.Delete(projectKey(id), present, nil)
	}
	writes := []types.TransactWriteItem{first}
	for _, rgm := range it.Members {
		writes = append(writes, s.leave(rgm, id))
	}
	if err := s.table.Transact(ctx, writes...); err != nil {
		if dynamo.FailedAt(err, 0) {
			return errStale
		}
		return err
	}

	items, err := s.table.QueryAll(ctx, &dynamodb.QueryInput{
		KeyConditionExpression:    aws.String("PK = :pk"),
		ExpressionAttributeValues: dynamo.Item{":pk": dynamo.S(projectPrefix + id)},
		ProjectionExpression:      aws.String("PK, SK"),
	})
	if err != nil {
		return fmt.Errorf("project deleted, listing its items: %w", err)
	}
	keys := make([]dynamo.Item, 0, len(items))
	for _, i := range items {
		keys = append(keys, dynamo.Item{"PK": i["PK"], "SK": i["SK"]})
	}
	if err := s.table.DeleteKeys(ctx, keys); err != nil {
		return fmt.Errorf("project deleted, deleting its items: %w", err)
	}
	return nil
}

func (s *DynamoStore) CreateInvitation(ctx context.Context, inv Invitation) error {
	item, err := attributevalue.MarshalMap(invitationItem{
		PK: projectPrefix + inv.ProjectID, SK: invitationPrefix + inv.RGM,
		GSI1PK: invitationPrefix + inv.RGM, GSI1SK: projectPrefix + inv.ProjectID,
		ProjectName: inv.ProjectName, InvitedBy: inv.InvitedBy, CreatedAt: inv.CreatedAt,
	})
	if err != nil {
		return err
	}
	_, err = s.table.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.table.Name),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(SK)"),
	})
	if dynamo.ConditionFailed(err) {
		return errDuplicate
	}
	return err
}

func (s *DynamoStore) InvitationExists(ctx context.Context, projectID, rgm string) (bool, error) {
	_, found, err := dynamo.Get[invitationItem](ctx, s.table, invitationKey(projectID, rgm))
	return found, err
}

func (s *DynamoStore) InvitationsOfProject(ctx context.Context, projectID string) ([]Invitation, error) {
	return invitations(dynamo.QueryPartition[invitationItem](ctx, s.table, projectPrefix+projectID, invitationPrefix))
}

func (s *DynamoStore) InvitationsOfStudent(ctx context.Context, rgm string) ([]Invitation, error) {
	return invitations(dynamo.QueryIndex[invitationItem](ctx, s.table, invitationPrefix+rgm))
}

func invitations(items []invitationItem, err error) ([]Invitation, error) {
	if err != nil {
		return nil, err
	}
	out := make([]Invitation, 0, len(items))
	for _, it := range items {
		out = append(out, it.invitation())
	}
	return out, nil
}

func (s *DynamoStore) DeleteInvitation(ctx context.Context, projectID, rgm string) error {
	_, err := s.table.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(s.table.Name),
		Key:                 invitationKey(projectID, rgm),
		ConditionExpression: aws.String("attribute_exists(SK)"),
	})
	if dynamo.ConditionFailed(err) {
		return errNotFound
	}
	return err
}
