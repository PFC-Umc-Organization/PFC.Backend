package references

import (
	"context"
	"errors"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
)

var (
	errNotFound  = errors.New("not found")
	errDuplicate = errors.New("already exists")
)

type Store interface {
	PutReference(ctx context.Context, r Reference, addedByID string) error
	ReferencesOf(ctx context.Context, projectID string) ([]Reference, error)
	DeleteReference(ctx context.Context, projectID, id string) error
	// ReplaceReference swaps reference oldID for r at once: errNotFound when
	// oldID is gone, errDuplicate when r is already on the list.
	ReplaceReference(ctx context.Context, oldID string, r Reference, addedByID string) error
}

// Works returns errWorkNotFound for an unknown DOI; any other error means
// the external service failed.
type Works interface {
	Search(ctx context.Context, term string, page int) (total int, works []Work, err error)
	WorkByDOI(ctx context.Context, doi string) (Work, error)
}

type Projects interface {
	Project(ctx context.Context, id string) (groups.Project, bool, error)
}
