package materials

import (
	"context"
	"errors"
	"log/slog"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/storage"
)

const (
	uploadTTL   = 15 * time.Minute
	downloadTTL = 5 * time.Minute
)

type Service struct {
	store    Store
	academic Academic
	files    Files
	newID    func() string
	now      func() time.Time
}

func NewService(store Store, academic Academic, files Files) *Service {
	return &Service{store: store, academic: academic, files: files, newID: uuid.NewString, now: time.Now}
}

func resource(id string) *audit.Resource { return &audit.Resource{Type: "material", ID: id} }

// cohortOf is the student's cohort; staff need none.
func (s *Service) cohortOf(ctx context.Context, viewer actor.Actor) (string, error) {
	if isStaff(viewer) {
		return "", nil
	}
	return s.academic.CohortOf(ctx, viewer.RGM())
}

// List puts the newest first.
func (s *Service) List(ctx context.Context, viewer actor.Actor) ([]Material, error) {
	cohort, err := s.cohortOf(ctx, viewer)
	if err != nil {
		return nil, apperr.Internal("Não foi possível carregar os materiais. Tente novamente em instantes.", err)
	}
	all, err := s.store.List(ctx)
	if err != nil {
		return nil, apperr.Internal("Não foi possível carregar os materiais. Tente novamente em instantes.", err)
	}
	visible := slices.DeleteFunc(all, func(m Material) bool { return !m.visibleTo(viewer, cohort) })
	slices.SortStableFunc(visible, func(a, b Material) int { return strings.Compare(b.PublishedAt, a.PublishedAt) })
	return visible, nil
}

func (s *Service) Create(ctx context.Context, author actor.Actor, in MaterialInput) (Material, error) {
	in = in.normalized()
	if msg := in.validate(); msg != "" {
		return Material{}, apperr.Invalid(msg)
	}
	if in.CohortID != nil {
		exists, err := s.academic.CohortExists(ctx, *in.CohortID)
		if err != nil {
			return Material{}, apperr.Internal("Não foi possível validar a turma. Tente novamente em instantes.", err)
		}
		if !exists {
			return Material{}, apperr.NotFound(msgCohortNotFound)
		}
	}
	m := Material{
		ID:          s.newID(),
		Title:       in.Title,
		Description: in.Description,
		Kind:        in.Kind,
		URL:         in.URL,
		CohortID:    in.CohortID,
		PublishedAt: s.now().UTC().Format(time.RFC3339),
		PublishedBy: author.Name,
		AuthorID:    author.Sub,
	}
	if in.File != "" {
		if s.files == nil {
			return Material{}, apperr.Unavailable(msgNoStorage, nil)
		}
		size, found, err := s.files.Size(ctx, in.File)
		if err != nil {
			return Material{}, apperr.Internal("Não foi possível conferir o arquivo enviado. Tente novamente em instantes.", err)
		}
		if !found || size > storage.MaxBytes {
			return Material{}, apperr.Invalid(msgFileMissing)
		}
		m.fileKey, m.File = in.File, path.Base(in.File)
	}
	if err := s.store.Create(ctx, m); err != nil {
		return Material{}, apperr.Internal("Não foi possível publicar o material. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: "material.publicado", Result: audit.Success, Resource: resource(m.ID),
		Details: map[string]any{"titulo": m.Title, "tipo": string(m.Kind), "cursoId": m.CohortID}})
	return m, nil
}

func (s *Service) Delete(ctx context.Context, who actor.Actor, id string) error {
	m, found, err := s.store.Get(ctx, id)
	if err != nil {
		return apperr.Internal("Não foi possível carregar o material. Tente novamente em instantes.", err)
	}
	if !found {
		return apperr.NotFound(msgNotFound)
	}
	if !m.canRemove(who) {
		audit.Record(ctx, audit.Event{Action: "material.removido", Result: audit.Failure, Reason: msgRemoveOwn, Resource: resource(id)})
		return apperr.Forbidden(msgRemoveOwn)
	}
	removed, err := s.store.Delete(ctx, id)
	if errors.Is(err, errNotFound) {
		return apperr.NotFound(msgNotFound)
	}
	if err != nil {
		return apperr.Internal("Não foi possível remover o material. Tente novamente em instantes.", err)
	}
	// A file left behind costs little and must not undo the removal.
	if removed.fileKey != "" && s.files != nil {
		if err := s.files.Delete(ctx, removed.fileKey); err != nil {
			slog.WarnContext(ctx, "file not deleted", "key", removed.fileKey, "error", err)
		}
	}
	audit.Record(ctx, audit.Event{Action: "material.removido", Result: audit.Success, Resource: resource(id)})
	return nil
}

func (s *Service) RequestUpload(ctx context.Context, in UploadRequest) (UploadTicket, error) {
	if s.files == nil {
		return UploadTicket{}, apperr.Unavailable(msgNoStorage, nil)
	}
	switch {
	case in.Size <= 0:
		return UploadTicket{}, apperr.Invalid(msgFileEmpty)
	case in.Size > storage.MaxBytes:
		return UploadTicket{}, apperr.Invalid(msgFileSize)
	}
	name := storage.SafeName(in.Name)
	kind, ok := storage.ContentType(name)
	if !ok {
		return UploadTicket{}, apperr.Invalid(msgFileType)
	}
	key := filesPrefix + s.newID() + "/" + name
	up, err := s.files.PresignUpload(ctx, key, kind, storage.MaxBytes, uploadTTL)
	if err != nil {
		return UploadTicket{}, apperr.Internal("Não foi possível preparar o envio do arquivo. Tente novamente em instantes.", err)
	}
	return UploadTicket{URL: up.URL, Fields: up.Fields, Key: key, ExpiresAt: up.ExpiresAt.UTC().Format(time.RFC3339), MaxBytes: storage.MaxBytes}, nil
}

func (s *Service) Download(ctx context.Context, viewer actor.Actor, id string) (Download, error) {
	if s.files == nil {
		return Download{}, apperr.Unavailable(msgNoStorage, nil)
	}
	m, found, err := s.store.Get(ctx, id)
	if err != nil {
		return Download{}, apperr.Internal("Não foi possível carregar o material. Tente novamente em instantes.", err)
	}
	cohort, err := s.cohortOf(ctx, viewer)
	if err != nil {
		return Download{}, apperr.Internal("Não foi possível carregar o material. Tente novamente em instantes.", err)
	}
	if !found || !m.visibleTo(viewer, cohort) {
		return Download{}, apperr.NotFound(msgNotFound)
	}
	if m.fileKey == "" {
		return Download{}, apperr.NotFound(msgNoFile)
	}
	url, err := s.files.PresignDownload(ctx, m.fileKey, m.File, downloadTTL)
	if err != nil {
		return Download{}, apperr.Internal("Não foi possível preparar o download. Tente novamente em instantes.", err)
	}
	return Download{URL: url, Name: m.File}, nil
}
