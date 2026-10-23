package materials

import (
	"context"
	"errors"
	"time"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/storage"
)

var errNotFound = errors.New("not found")

type Store interface {
	Create(ctx context.Context, m Material) error
	Get(ctx context.Context, id string) (Material, bool, error)
	List(ctx context.Context) ([]Material, error)
	// Delete returns what was deleted, so its file can go too.
	Delete(ctx context.Context, id string) (Material, error)
}

type Academic interface {
	CohortExists(ctx context.Context, id string) (bool, error)
	CohortOf(ctx context.Context, rgm string) (string, error)
}

// Files is nil while no bucket is configured; materials are then links only.
type Files interface {
	PresignUpload(ctx context.Context, key, contentType string, maxBytes int64, ttl time.Duration) (storage.Upload, error)
	PresignDownload(ctx context.Context, key, fileName string, ttl time.Duration) (string, error)
	Size(ctx context.Context, key string) (int64, bool, error)
	Delete(ctx context.Context, key string) error
}
