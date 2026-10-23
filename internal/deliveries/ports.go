package deliveries

import (
	"context"
	"errors"
	"time"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/storage"
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
	GetSubmission(ctx context.Context, projectID, activityID string) (Submission, bool, error)
	// DeleteSubmission returns what was deleted, so its files can go too.
	DeleteSubmission(ctx context.Context, projectID, activityID string) (Submission, error)
	SubmissionsOfProject(ctx context.Context, projectID string) ([]Submission, error)
	SubmissionsOfActivity(ctx context.Context, activityID string) ([]Submission, error)
}

// Files is nil while no bucket is configured; file fields then keep only
// the file name, as before.
type Files interface {
	PresignUpload(ctx context.Context, key, contentType string, maxBytes int64, ttl time.Duration) (storage.Upload, error)
	PresignDownload(ctx context.Context, key, fileName string, ttl time.Duration) (string, error)
	Size(ctx context.Context, key string) (int64, bool, error)
	Delete(ctx context.Context, key string) error
}

type Projects interface {
	Project(ctx context.Context, id string) (groups.Project, bool, error)
	ProjectsOfProgram(ctx context.Context, viewer actor.Actor, programID string) ([]groups.Project, error)
}
