package references

import (
	"context"
	"errors"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
)

// ---------------------------------------------------------------------------
// Erros
// ---------------------------------------------------------------------------

var (
	errNotFound  = errors.New("not found")
	errDuplicate = errors.New("already exists")
)

// ---------------------------------------------------------------------------
// Repositório
// ---------------------------------------------------------------------------

// Repositório
type Store interface {
	PutReference(ctx context.Context, r Reference, addedByID string) error
	ReferencesOf(ctx context.Context, projectID string) ([]Reference, error)
	DeleteReference(ctx context.Context, projectID, id string) error
}

// ---------------------------------------------------------------------------
// Dependências
// ---------------------------------------------------------------------------

// Dependência
type Works interface {
	Search(ctx context.Context, term string, page int) (total int, works []Work, err error)
	WorkByDOI(ctx context.Context, doi string) (Work, error)
}

// Dependência
type Projects interface {
	Project(ctx context.Context, id string) (groups.Project, bool, error)
}
