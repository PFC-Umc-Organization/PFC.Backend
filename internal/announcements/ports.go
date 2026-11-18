package announcements

import (
	"context"
	"errors"
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
	Create(ctx context.Context, a Announcement) error
	Get(ctx context.Context, id string) (Announcement, bool, error)
	List(ctx context.Context) ([]Announcement, error)
	Delete(ctx context.Context, id string) error
}

// ---------------------------------------------------------------------------
// Dependências
// ---------------------------------------------------------------------------

// Dependência
type Academic interface {
	CohortExists(ctx context.Context, id string) (bool, error)
	CohortOf(ctx context.Context, rgm string) (string, error)
	CohortsOf(ctx context.Context, rgms []string) (map[string]string, error)
}

// Dependência
type Advising interface {
	MembersAdvisedBy(ctx context.Context, advisorSub string) ([]string, error)
}
