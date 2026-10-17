package deliveries

import (
	"context"
	"errors"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
)

var (
	errNotFound = errors.New("not found")
	errFull     = errors.New("field limit reached")
	errStale    = errors.New("item changed since it was read")
)

type Store interface {
	CreateActivity(ctx context.Context, a Activity) error
	GetActivity(ctx context.Context, id string) (Activity, bool, error)
	ListActivities(ctx context.Context) ([]Activity, error)
	UpdateActivity(ctx context.Context, id string, in ActivityInput) (Activity, error)
	DeleteActivity(ctx context.Context, id string) error
	AddField(ctx context.Context, activityID string, f Field) error
	RemoveField(ctx context.Context, activityID string, position int, fieldID string) error

	PutSubmission(ctx context.Context, s Submission) error
	DeleteSubmission(ctx context.Context, projectID, activityID string) error
	SubmissionsOfProject(ctx context.Context, projectID string) ([]Submission, error)
	SubmissionsOfActivity(ctx context.Context, activityID string) ([]Submission, error)
}

type Projects interface {
	Project(ctx context.Context, id string) (groups.Project, bool, error)
}
