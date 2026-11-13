package deliveries

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
//	activity    PK=ACTIVITY#<id>  SK=PROFILE        GSI1PK=ACTIVITIES      GSI1SK=ACTIVITY#<id>  campos (list)
//	submission  PK=PROJECT#<pid>  SK=ENTREGA#<aid>  GSI1PK=ENTREGAS#<aid>  GSI1SK=PROJECT#<pid>
//	penalty     PK=PROJECT#<pid>  SK=PENALIDADE#<id>  (manual penalty or waiver)
//
// A submission lives in the project's partition (a group's submissions are
// one Query) and the GSI lists an activity's submissions.
const (
	activityPrefix      = "ACTIVITY#"
	projectPrefix       = "PROJECT#"
	submissionPrefix    = "ENTREGA#"
	submissionsPrefix   = "ENTREGAS#"
	penaltyPrefix       = "PENALIDADE#"
	activitiesPartition = "ACTIVITIES"
	profileSK           = "PROFILE"
)

type DynamoStore struct {
	table *dynamo.Table
}

func NewDynamoStore(t *dynamo.Table) *DynamoStore { return &DynamoStore{table: t} }

type fieldItem struct {
	ID       string    `dynamodbav:"id"`
	Label    string    `dynamodbav:"rotulo"`
	Type     FieldType `dynamodbav:"tipo"`
	Required bool      `dynamodbav:"obrigatorio"`
}

type activityItem struct {
	PK          string      `dynamodbav:"PK"`
	SK          string      `dynamodbav:"SK"`
	GSI1PK      string      `dynamodbav:"GSI1PK"`
	GSI1SK      string      `dynamodbav:"GSI1SK"`
	Title       string      `dynamodbav:"titulo"`
	Description string      `dynamodbav:"descricao"`
	Deadline    string      `dynamodbav:"prazo"`
	PublishedAt string      `dynamodbav:"publicadaEm"`
	Fields      []fieldItem `dynamodbav:"campos"`
	CohortID    string      `dynamodbav:"turmaId,omitempty"`
}

func (it activityItem) activity() Activity {
	fields := make([]Field, 0, len(it.Fields))
	for _, f := range it.Fields {
		fields = append(fields, Field(f))
	}
	return Activity{
		ID:          strings.TrimPrefix(it.PK, activityPrefix),
		Title:       it.Title,
		Description: it.Description,
		Deadline:    it.Deadline,
		PublishedAt: it.PublishedAt,
		CohortID:    it.CohortID,
		Fields:      fields,
	}
}

type submissionItem struct {
	PK          string            `dynamodbav:"PK"`
	SK          string            `dynamodbav:"SK"`
	GSI1PK      string            `dynamodbav:"GSI1PK"`
	GSI1SK      string            `dynamodbav:"GSI1SK"`
	SubmittedAt string            `dynamodbav:"entregueEm"`
	SubmittedBy string            `dynamodbav:"entreguePor"`
	UpdatedAt   string            `dynamodbav:"atualizadaEm,omitempty"`
	UpdatedBy   string            `dynamodbav:"atualizadaPor,omitempty"`
	Answers     map[string]string `dynamodbav:"respostas"`
	Validation  *validationItem   `dynamodbav:"validacao,omitempty"`
}

type validationItem struct {
	Status  ValidationStatus `dynamodbav:"status"`
	Comment string           `dynamodbav:"comentario,omitempty"`
	By      string           `dynamodbav:"por"`
	ByName  string           `dynamodbav:"porNome"`
	At      string           `dynamodbav:"em"`
}

func (it submissionItem) submission() Submission {
	answers := it.Answers
	if answers == nil {
		answers = map[string]string{}
	}
	sub := Submission{
		ActivityID:  strings.TrimPrefix(it.SK, submissionPrefix),
		ProjectID:   strings.TrimPrefix(it.PK, projectPrefix),
		SubmittedAt: it.SubmittedAt,
		SubmittedBy: it.SubmittedBy,
		UpdatedAt:   it.UpdatedAt,
		UpdatedBy:   it.UpdatedBy,
		Answers:     answers,
	}
	if it.Validation != nil {
		v := Validation(*it.Validation)
		sub.Validation = &v
	}
	return sub
}

func activityKey(id string) dynamo.Item { return dynamo.Key(activityPrefix+id, profileSK) }
func submissionKey(projectID, activityID string) dynamo.Item {
	return dynamo.Key(projectPrefix+projectID, submissionPrefix+activityID)
}

func (s *DynamoStore) CreateActivity(ctx context.Context, a Activity) error {
	fields := make([]fieldItem, 0, len(a.Fields))
	for _, f := range a.Fields {
		fields = append(fields, fieldItem(f))
	}
	err := dynamo.PutStruct(ctx, s.table, activityItem{
		PK: activityPrefix + a.ID, SK: profileSK, GSI1PK: activitiesPartition, GSI1SK: activityPrefix + a.ID,
		Title: a.Title, Description: a.Description, Deadline: a.Deadline, PublishedAt: a.PublishedAt, Fields: fields, CohortID: a.CohortID,
	}, dynamo.Write{Condition: "attribute_not_exists(PK)"})
	if dynamo.ConditionFailed(err) {
		return errDuplicate
	}
	return err
}

func (s *DynamoStore) GetActivity(ctx context.Context, id string) (Activity, bool, error) {
	it, found, err := dynamo.Get[activityItem](ctx, s.table, activityKey(id))
	if err != nil || !found {
		return Activity{}, false, err
	}
	return it.activity(), true, nil
}

func (s *DynamoStore) ListActivities(ctx context.Context) ([]Activity, error) {
	items, err := dynamo.QueryIndex[activityItem](ctx, s.table, activitiesPartition)
	if err != nil {
		return nil, err
	}
	out := make([]Activity, 0, len(items))
	for _, it := range items {
		out = append(out, it.activity())
	}
	return out, nil
}

// UpdateActivity returns the updated item (ALL_NEW), saving a second read.
func (s *DynamoStore) UpdateActivity(ctx context.Context, id string, in ActivityInput) (Activity, error) {
	attrs, err := s.table.UpdateItem(ctx, activityKey(id), "SET titulo = :t, descricao = :d, prazo = :p", dynamo.Write{
		Condition: "attribute_exists(PK)",
		Values: dynamo.Item{
			":t": dynamo.S(in.Title), ":d": dynamo.S(in.Description), ":p": dynamo.S(in.Deadline),
		},
		Return: types.ReturnValueAllNew,
	})
	if dynamo.ConditionFailed(err) {
		return Activity{}, errNotFound
	}
	if err != nil {
		return Activity{}, err
	}
	var it activityItem
	if err := attributevalue.UnmarshalMap(attrs, &it); err != nil {
		return Activity{}, err
	}
	return it.activity(), nil
}

func (s *DynamoStore) DeleteActivity(ctx context.Context, id string) ([]string, error) {
	_, err := s.table.DeleteItem(ctx, activityKey(id), dynamo.Write{Condition: "attribute_exists(PK)"})
	if dynamo.ConditionFailed(err) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	items, err := s.table.QueryAll(ctx, &dynamodb.QueryInput{
		IndexName:                 aws.String(s.table.Index),
		KeyConditionExpression:    aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: dynamo.Item{":pk": dynamo.S(submissionsPrefix + id)},
		ProjectionExpression:      aws.String("PK, SK"),
	})
	if err != nil {
		return nil, err
	}
	keys := make([]dynamo.Item, 0, len(items))
	projects := make([]string, 0, len(items))
	for _, i := range items {
		keys = append(keys, dynamo.Item{"PK": i["PK"], "SK": i["SK"]})
		if pk, ok := i["PK"].(*types.AttributeValueMemberS); ok {
			projects = append(projects, strings.TrimPrefix(pk.Value, projectPrefix))
		}
	}
	return projects, s.table.DeleteKeys(ctx, keys)
}

func (s *DynamoStore) AddField(ctx context.Context, activityID string, f Field) error {
	value, err := attributevalue.Marshal(fieldItem(f))
	if err != nil {
		return err
	}
	_, err = s.table.UpdateItem(ctx, activityKey(activityID), "SET campos = list_append(campos, :f)", dynamo.Write{
		Condition: "attribute_exists(PK) AND size(campos) < :max",
		Values: dynamo.Item{
			":f":   &types.AttributeValueMemberL{Value: []types.AttributeValue{value}},
			":max": dynamo.N(maxFields),
		},
		// On rejection DynamoDB returns the item as it was, which tells a
		// missing activity from a full one without a second read.
		OldOnFail: true,
	})
	var rejected *dynamo.ConditionFailedError
	if errors.As(err, &rejected) {
		if len(rejected.Old) > 0 {
			return errFull
		}
		return errNotFound
	}
	return err
}

// RemoveField conditions on the field still being at position, so a
// concurrent change surfaces as errStale.
func (s *DynamoStore) RemoveField(ctx context.Context, activityID string, position int, fieldID string) error {
	_, err := s.table.UpdateItem(ctx, activityKey(activityID), fmt.Sprintf("REMOVE campos[%d]", position), dynamo.Write{
		Condition: fmt.Sprintf("size(campos) > :one AND campos[%d].id = :id", position),
		Values:    dynamo.Item{":one": dynamo.N(1), ":id": dynamo.S(fieldID)},
	})
	if dynamo.ConditionFailed(err) {
		return errStale
	}
	return err
}

// projectProfileSK mirrors the project item's sort key in groups/store.go;
// deliveries cannot import that package's store.
const projectProfileSK = "PROFILE"

func (s *DynamoStore) PutSubmission(ctx context.Context, sub Submission, expected *Submission) error {
	item, err := attributevalue.MarshalMap(submissionItem{
		PK: projectPrefix + sub.ProjectID, SK: submissionPrefix + sub.ActivityID,
		GSI1PK: submissionsPrefix + sub.ActivityID, GSI1SK: projectPrefix + sub.ProjectID,
		SubmittedAt: sub.SubmittedAt, SubmittedBy: sub.SubmittedBy,
		UpdatedAt: sub.UpdatedAt, UpdatedBy: sub.UpdatedBy, Answers: sub.Answers,
	})
	if err != nil {
		return err
	}
	condition, values := "attribute_not_exists(PK)", dynamo.Item(nil)
	if expected != nil {
		condition = "attribute_exists(PK) AND entregueEm = :sentAt AND attribute_not_exists(atualizadaEm)"
		values = dynamo.Item{":sentAt": dynamo.S(expected.SubmittedAt)}
		if expected.UpdatedAt != "" {
			condition = "attribute_exists(PK) AND entregueEm = :sentAt AND atualizadaEm = :updatedAt"
			values[":updatedAt"] = dynamo.S(expected.UpdatedAt)
		}
	}
	err = s.table.Transact(ctx,
		s.table.Check(dynamo.Key(projectPrefix+sub.ProjectID, projectProfileSK), "attribute_exists(PK)", nil),
		s.table.Put(item, condition, values),
	)
	switch {
	case dynamo.FailedAt(err, 0):
		return errNotFound
	case dynamo.FailedAt(err, 1):
		return errStale
	}
	return err
}

// SetValidation only touches a submission still there; the group's next
// edit replaces the whole item, which drops the validation with it.
func (s *DynamoStore) SetValidation(ctx context.Context, projectID, activityID string, v Validation) (Submission, error) {
	value, err := attributevalue.Marshal(validationItem(v))
	if err != nil {
		return Submission{}, err
	}
	attrs, err := s.table.UpdateItem(ctx, submissionKey(projectID, activityID), "SET validacao = :v", dynamo.Write{
		Condition: "attribute_exists(PK)",
		Values:    dynamo.Item{":v": value},
		Return:    types.ReturnValueAllNew,
	})
	if dynamo.ConditionFailed(err) {
		return Submission{}, errNotFound
	}
	if err != nil {
		return Submission{}, err
	}
	var it submissionItem
	if err := attributevalue.UnmarshalMap(attrs, &it); err != nil {
		return Submission{}, err
	}
	return it.submission(), nil
}

func (s *DynamoStore) GetSubmission(ctx context.Context, projectID, activityID string) (Submission, bool, error) {
	it, found, err := dynamo.Get[submissionItem](ctx, s.table, submissionKey(projectID, activityID))
	if err != nil || !found {
		return Submission{}, found, err
	}
	return it.submission(), true, nil
}

func (s *DynamoStore) DeleteSubmission(ctx context.Context, projectID, activityID string) (Submission, error) {
	old, err := s.table.DeleteItem(ctx, submissionKey(projectID, activityID), dynamo.Write{Condition: "attribute_exists(PK)", Return: types.ReturnValueAllOld})
	if dynamo.ConditionFailed(err) {
		return Submission{}, errNotFound
	}
	if err != nil {
		return Submission{}, err
	}
	var it submissionItem
	if err := attributevalue.UnmarshalMap(old, &it); err != nil {
		return Submission{}, err
	}
	return it.submission(), nil
}

func (s *DynamoStore) SubmissionsOfProject(ctx context.Context, projectID string) ([]Submission, error) {
	return submissions(dynamo.QueryPartition[submissionItem](ctx, s.table, projectPrefix+projectID, submissionPrefix))
}

func (s *DynamoStore) SubmissionsOfActivity(ctx context.Context, activityID string) ([]Submission, error) {
	return submissions(dynamo.QueryIndex[submissionItem](ctx, s.table, submissionsPrefix+activityID))
}

func submissions(items []submissionItem, err error) ([]Submission, error) {
	if err != nil {
		return nil, err
	}
	out := make([]Submission, 0, len(items))
	for _, it := range items {
		out = append(out, it.submission())
	}
	return out, nil
}

type penaltyItem struct {
	PK         string `dynamodbav:"PK"`
	SK         string `dynamodbav:"SK"`
	ActivityID string `dynamodbav:"atividadeId,omitempty"`
	Reason     string `dynamodbav:"motivo,omitempty"`
	AppliedAt  string `dynamodbav:"aplicadaEm,omitempty"`
	AppliedBy  string `dynamodbav:"aplicadaPor,omitempty"`
	WaivedFor  string `dynamodbav:"perdaoMotivo,omitempty"`
	WaivedBy   string `dynamodbav:"perdoadaPor,omitempty"`
	WaivedAt   string `dynamodbav:"perdoadaEm,omitempty"`
}

func (it penaltyItem) record() penaltyRecord {
	r := penaltyRecord{
		ID:         strings.TrimPrefix(it.SK, penaltyPrefix),
		ActivityID: it.ActivityID,
		Reason:     it.Reason,
		AppliedAt:  it.AppliedAt,
		AppliedBy:  it.AppliedBy,
	}
	if it.WaivedAt != "" {
		r.Waiver = &Waiver{Reason: it.WaivedFor, By: it.WaivedBy, At: it.WaivedAt}
	}
	return r
}

func (s *DynamoStore) PenaltiesOfProject(ctx context.Context, projectID string) ([]penaltyRecord, error) {
	items, err := dynamo.QueryPartition[penaltyItem](ctx, s.table, projectPrefix+projectID, penaltyPrefix)
	if err != nil {
		return nil, err
	}
	out := make([]penaltyRecord, 0, len(items))
	for _, it := range items {
		out = append(out, it.record())
	}
	return out, nil
}

func (s *DynamoStore) AddPenalty(ctx context.Context, projectID string, r penaltyRecord) error {
	err := dynamo.PutStruct(ctx, s.table, penaltyItem{
		PK: projectPrefix + projectID, SK: penaltyPrefix + r.ID,
		ActivityID: r.ActivityID, Reason: r.Reason, AppliedAt: r.AppliedAt, AppliedBy: r.AppliedBy,
	}, dynamo.Write{Condition: "attribute_not_exists(PK)"})
	if dynamo.ConditionFailed(err) {
		return errDuplicate
	}
	return err
}

// WaivePenalty creates the item for a deadline penalty, which is not stored
// until waived.
func (s *DynamoStore) WaivePenalty(ctx context.Context, projectID, penaltyID string, w Waiver) error {
	_, err := s.table.UpdateItem(ctx, dynamo.Key(projectPrefix+projectID, penaltyPrefix+penaltyID),
		"SET perdaoMotivo = :motivo, perdoadaPor = :por, perdoadaEm = :em", dynamo.Write{
			Condition: "attribute_not_exists(perdoadaEm)",
			Values: dynamo.Item{
				":motivo": dynamo.S(w.Reason), ":por": dynamo.S(w.By), ":em": dynamo.S(w.At),
			},
		})
	if dynamo.ConditionFailed(err) {
		return errStale
	}
	return err
}
