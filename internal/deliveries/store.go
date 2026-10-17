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
//
// A submission lives in the project's partition (a group's submissions are
// one Query) and the GSI lists an activity's submissions.
const (
	activityPrefix      = "ACTIVITY#"
	projectPrefix       = "PROJECT#"
	submissionPrefix    = "ENTREGA#"
	submissionsPrefix   = "ENTREGAS#"
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
	Answers     map[string]string `dynamodbav:"respostas"`
}

func (it submissionItem) submission() Submission {
	answers := it.Answers
	if answers == nil {
		answers = map[string]string{}
	}
	return Submission{
		ActivityID:  strings.TrimPrefix(it.SK, submissionPrefix),
		ProjectID:   strings.TrimPrefix(it.PK, projectPrefix),
		SubmittedAt: it.SubmittedAt,
		SubmittedBy: it.SubmittedBy,
		Answers:     answers,
	}
}

func activityKey(id string) dynamo.Item { return dynamo.Key(activityPrefix+id, profileSK) }
func submissionKey(projectID, activityID string) dynamo.Item {
	return dynamo.Key(projectPrefix+projectID, submissionPrefix+activityID)
}

func (s *DynamoStore) put(ctx context.Context, v any) error {
	item, err := attributevalue.MarshalMap(v)
	if err != nil {
		return err
	}
	_, err = s.table.Client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(s.table.Name), Item: item})
	return err
}

func (s *DynamoStore) CreateActivity(ctx context.Context, a Activity) error {
	fields := make([]fieldItem, 0, len(a.Fields))
	for _, f := range a.Fields {
		fields = append(fields, fieldItem(f))
	}
	return s.put(ctx, activityItem{
		PK: activityPrefix + a.ID, SK: profileSK, GSI1PK: activitiesPartition, GSI1SK: activityPrefix + a.ID,
		Title: a.Title, Description: a.Description, Deadline: a.Deadline, PublishedAt: a.PublishedAt, Fields: fields,
	})
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
	out, err := s.table.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(s.table.Name),
		Key:                 activityKey(id),
		UpdateExpression:    aws.String("SET titulo = :t, descricao = :d, prazo = :p"),
		ConditionExpression: aws.String("attribute_exists(PK)"),
		ExpressionAttributeValues: dynamo.Item{
			":t": dynamo.S(in.Title), ":d": dynamo.S(in.Description), ":p": dynamo.S(in.Deadline),
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if dynamo.ConditionFailed(err) {
		return Activity{}, errNotFound
	}
	if err != nil {
		return Activity{}, err
	}
	var it activityItem
	if err := attributevalue.UnmarshalMap(out.Attributes, &it); err != nil {
		return Activity{}, err
	}
	return it.activity(), nil
}

func (s *DynamoStore) DeleteActivity(ctx context.Context, id string) error {
	_, err := s.table.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(s.table.Name),
		Key:                 activityKey(id),
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})
	if dynamo.ConditionFailed(err) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	items, err := s.table.QueryAll(ctx, &dynamodb.QueryInput{
		IndexName:                 aws.String(s.table.Index),
		KeyConditionExpression:    aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: dynamo.Item{":pk": dynamo.S(submissionsPrefix + id)},
		ProjectionExpression:      aws.String("PK, SK"),
	})
	if err != nil {
		return err
	}
	keys := make([]dynamo.Item, 0, len(items))
	for _, i := range items {
		keys = append(keys, dynamo.Item{"PK": i["PK"], "SK": i["SK"]})
	}
	return s.table.DeleteKeys(ctx, keys)
}

func (s *DynamoStore) AddField(ctx context.Context, activityID string, f Field) error {
	value, err := attributevalue.Marshal(fieldItem(f))
	if err != nil {
		return err
	}
	_, err = s.table.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(s.table.Name),
		Key:                 activityKey(activityID),
		UpdateExpression:    aws.String("SET campos = list_append(campos, :f)"),
		ConditionExpression: aws.String("attribute_exists(PK) AND size(campos) < :max"),
		ExpressionAttributeValues: dynamo.Item{
			":f":   &types.AttributeValueMemberL{Value: []types.AttributeValue{value}},
			":max": dynamo.N(maxFields),
		},
		// On rejection DynamoDB returns the item as it was, which tells a
		// missing activity from a full one without a second read.
		ReturnValuesOnConditionCheckFailure: types.ReturnValuesOnConditionCheckFailureAllOld,
	})
	var rejected *types.ConditionalCheckFailedException
	if errors.As(err, &rejected) {
		if len(rejected.Item) > 0 {
			return errFull
		}
		return errNotFound
	}
	return err
}

// RemoveField conditions on the field still being at position, so a
// concurrent change surfaces as errStale.
func (s *DynamoStore) RemoveField(ctx context.Context, activityID string, position int, fieldID string) error {
	_, err := s.table.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.table.Name),
		Key:                       activityKey(activityID),
		UpdateExpression:          aws.String(fmt.Sprintf("REMOVE campos[%d]", position)),
		ConditionExpression:       aws.String(fmt.Sprintf("size(campos) > :one AND campos[%d].id = :id", position)),
		ExpressionAttributeValues: dynamo.Item{":one": dynamo.N(1), ":id": dynamo.S(fieldID)},
	})
	if dynamo.ConditionFailed(err) {
		return errStale
	}
	return err
}

func (s *DynamoStore) PutSubmission(ctx context.Context, sub Submission) error {
	return s.put(ctx, submissionItem{
		PK: projectPrefix + sub.ProjectID, SK: submissionPrefix + sub.ActivityID,
		GSI1PK: submissionsPrefix + sub.ActivityID, GSI1SK: projectPrefix + sub.ProjectID,
		SubmittedAt: sub.SubmittedAt, SubmittedBy: sub.SubmittedBy, Answers: sub.Answers,
	})
}

func (s *DynamoStore) DeleteSubmission(ctx context.Context, projectID, activityID string) error {
	_, err := s.table.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(s.table.Name),
		Key:                 submissionKey(projectID, activityID),
		ConditionExpression: aws.String("attribute_exists(PK)"),
	})
	if dynamo.ConditionFailed(err) {
		return errNotFound
	}
	return err
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
