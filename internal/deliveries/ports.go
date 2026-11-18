package deliveries

import (
	"context"
	"errors"
	"time"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/storage"
)

// ---------------------------------------------------------------------------
// Erros
// ---------------------------------------------------------------------------

var (
	errNotFound  = errors.New("not found")
	errFull      = errors.New("field limit reached")
	errStale     = errors.New("item changed since it was read")
	errDuplicate = errors.New("already exists")
)

// ---------------------------------------------------------------------------
// Repositório
// ---------------------------------------------------------------------------

// Repositório
type Store interface {
	CreateActivity(ctx context.Context, a Activity) error
	GetActivity(ctx context.Context, id string) (Activity, bool, error)
	ListActivities(ctx context.Context) ([]Activity, error)
	UpdateActivity(ctx context.Context, id string, in ActivityInput) (Activity, error)

	DeleteActivity(ctx context.Context, id string) ([]string, error)
	AddField(ctx context.Context, activityID string, f Field) error
	RemoveField(ctx context.Context, activityID string, position int, fieldID string) error

	PutSubmission(ctx context.Context, s Submission, expected *Submission) error
	GetSubmission(ctx context.Context, projectID, activityID string) (Submission, bool, error)

	SetValidation(ctx context.Context, projectID, activityID string, v Validation) (Submission, error)

	DeleteSubmission(ctx context.Context, projectID, activityID string) (Submission, error)
	SubmissionsOfProject(ctx context.Context, projectID string) ([]Submission, error)
	SubmissionsOfActivity(ctx context.Context, activityID string) ([]Submission, error)

	PenaltiesOfProject(ctx context.Context, projectID string) ([]penaltyRecord, error)
	AddPenalty(ctx context.Context, projectID string, r penaltyRecord) error

	WaivePenalty(ctx context.Context, projectID, penaltyID string, w Waiver) error
}

// ---------------------------------------------------------------------------
// Dependências
// ---------------------------------------------------------------------------

// Dependência
type Files interface {
	PresignUpload(ctx context.Context, key, contentType string, maxBytes int64, ttl time.Duration) (storage.Upload, error)
	PresignDownload(ctx context.Context, key, fileName string, ttl time.Duration) (string, error)
	Size(ctx context.Context, key string) (int64, bool, error)
	DeleteKeys(ctx context.Context, keys []string) error
	DeletePrefix(ctx context.Context, prefix string) error
}

// Dependência
type Projects interface {
	Project(ctx context.Context, id string) (groups.Project, bool, error)
	ProjectsOfProgram(ctx context.Context, viewer actor.Actor, programID string) ([]groups.Project, error)
}

// Dependência
type Cohorts interface {
	OfProject(ctx context.Context, p groups.Project) (string, error)

	OfStudent(ctx context.Context, rgm string) (string, error)
	Exists(ctx context.Context, id string) (bool, error)
}
