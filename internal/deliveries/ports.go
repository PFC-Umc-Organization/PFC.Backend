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
	errNotFound  = errors.New("not found")
	errFull      = errors.New("field limit reached")
	errStale     = errors.New("item changed since it was read")
	errDuplicate = errors.New("already exists")
)

type Store interface {
	CreateActivity(ctx context.Context, a Activity) error
	GetActivity(ctx context.Context, id string) (Activity, bool, error)
	ListActivities(ctx context.Context) ([]Activity, error)
	UpdateActivity(ctx context.Context, id string, in ActivityInput) (Activity, error)
	// DeleteActivity returns the projects that had a submission, whose files
	// go too.
	DeleteActivity(ctx context.Context, id string) ([]string, error)
	AddField(ctx context.Context, activityID string, f Field) error
	RemoveField(ctx context.Context, activityID string, position int, fieldID string) error

	// PutSubmission creates the submission when expected is nil and replaces
	// it only if it is still the one that was read (expected), so a
	// concurrent change, or a hand-back after the deadline, is never
	// overwritten. It returns errStale when the stored submission is not
	// the expected one and errNotFound when the project is gone.
	PutSubmission(ctx context.Context, s Submission, expected *Submission) error
	GetSubmission(ctx context.Context, projectID, activityID string) (Submission, bool, error)
	// SetValidation returns errNotFound when the submission is gone.
	SetValidation(ctx context.Context, projectID, activityID string, v Validation) (Submission, error)
	// DeleteSubmission returns what was deleted, so its files can go too.
	DeleteSubmission(ctx context.Context, projectID, activityID string) (Submission, error)
	SubmissionsOfProject(ctx context.Context, projectID string) ([]Submission, error)
	SubmissionsOfActivity(ctx context.Context, activityID string) ([]Submission, error)

	PenaltiesOfProject(ctx context.Context, projectID string) ([]penaltyRecord, error)
	AddPenalty(ctx context.Context, projectID string, r penaltyRecord) error
	// WaivePenalty returns errStale when the penalty was already waived.
	WaivePenalty(ctx context.Context, projectID, penaltyID string, w Waiver) error
}

// Files is nil while no bucket is configured; file fields then keep only
// the file name, as before.
type Files interface {
	PresignUpload(ctx context.Context, key, contentType string, maxBytes int64, ttl time.Duration) (storage.Upload, error)
	PresignDownload(ctx context.Context, key, fileName string, ttl time.Duration) (string, error)
	Size(ctx context.Context, key string) (int64, bool, error)
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
}

type Projects interface {
	Project(ctx context.Context, id string) (groups.Project, bool, error)
	ProjectsOfProgram(ctx context.Context, viewer actor.Actor, programID string) ([]groups.Project, error)
}

// Cohorts is how deliveries learns the cohort of a group and of a student, and
// whether a cohort exists; the composition root adapts the academic module.
type Cohorts interface {
	// OfProject is empty when the group's program has no known cohort.
	OfProject(ctx context.Context, p groups.Project) (string, error)
	// OfStudent is empty when the RGM is not enrolled in any cohort.
	OfStudent(ctx context.Context, rgm string) (string, error)
	Exists(ctx context.Context, id string) (bool, error)
}
