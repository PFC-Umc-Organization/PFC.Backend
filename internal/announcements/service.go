package announcements

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

// Serviço
type Service struct {
	store    Store
	academic Academic
	advising Advising
	newID    func() string
	now      func() time.Time
}

// Construção
func NewService(store Store, academic Academic, advising Advising) *Service {
	return &Service{store: store, academic: academic, advising: advising, newID: uuid.NewString, now: time.Now}
}

// Auditoria
func resource(id string) *audit.Resource { return &audit.Resource{Type: "aviso", ID: id} }

// Consulta
func (s *Service) advisedCohorts(ctx context.Context, advisor actor.Actor) (map[string]bool, error) {
	members, err := s.advising.MembersAdvisedBy(ctx, advisor.Sub)
	if err != nil {
		return nil, fmt.Errorf("advised members: %w", err)
	}
	cohorts, err := s.academic.CohortsOf(ctx, members)
	if err != nil {
		return nil, fmt.Errorf("cohorts of members: %w", err)
	}
	set := map[string]bool{}
	for _, c := range cohorts {
		if c != "" {
			set[c] = true
		}
	}
	return set, nil
}

// Listagem
func (s *Service) List(ctx context.Context, viewer actor.Actor, cohortID string) ([]Announcement, error) {
	const msg = "Não foi possível carregar os avisos. Tente novamente em instantes."
	cohortID = strings.TrimSpace(cohortID)
	var visible map[string]bool
	switch {
	case viewer.HasRole(actor.RoleAdmin):
	case viewer.HasRole(actor.RoleAdvisor):
		set, err := s.advisedCohorts(ctx, viewer)
		if err != nil {
			return nil, apperr.Internal(msg, err)
		}
		visible = set
	default:
		cohort, err := s.academic.CohortOf(ctx, viewer.RGM())
		if err != nil {
			return nil, apperr.Internal(msg, fmt.Errorf("cohort of student: %w", err))
		}
		visible = map[string]bool{}
		if cohort != "" {
			visible[cohort] = true
		}
	}
	all, err := s.store.List(ctx)
	if err != nil {
		return nil, apperr.Internal(msg, fmt.Errorf("list announcements: %w", err))
	}
	all = slices.DeleteFunc(all, func(a Announcement) bool {
		return (visible != nil && !a.visibleIn(visible)) || (cohortID != "" && a.CohortID != cohortID)
	})
	slices.SortStableFunc(all, func(a, b Announcement) int { return strings.Compare(b.CreatedAt, a.CreatedAt) })
	return all, nil
}

// Publicação
func (s *Service) Create(ctx context.Context, author actor.Actor, in AnnouncementInput) (Announcement, error) {
	const action = "aviso.publicado"
	in = in.normalized()
	if msg := in.validate(); msg != "" {
		return Announcement{}, apperr.Invalid(msg)
	}
	if err := s.checkCohort(ctx, author, in.CohortID); err != nil {
		audit.Record(ctx, audit.Event{Action: action, Result: audit.Failure, Reason: err.Error()})
		return Announcement{}, err
	}
	a := Announcement{
		ID: s.newID(), Title: in.Title, Content: in.Content, CohortID: in.CohortID,
		AuthorID: author.Sub, AuthorName: author.Name, AuthorRole: string(author.Role),
		CreatedAt: s.now().UTC().Format(time.RFC3339),
	}
	if err := s.store.Create(ctx, a); errors.Is(err, errDuplicate) {
		return Announcement{}, apperr.Conflict("Já existe um aviso com este identificador. Tente novamente.")
	} else if err != nil {
		return Announcement{}, apperr.Internal("Não foi possível publicar o aviso. Tente novamente em instantes.", fmt.Errorf("create announcement: %w", err))
	}
	audit.Record(ctx, audit.Event{Action: action, Result: audit.Success, Resource: resource(a.ID),
		Details: map[string]any{"titulo": a.Title, "turmaId": a.CohortID}})
	return a, nil
}

// Validação
func (s *Service) checkCohort(ctx context.Context, author actor.Actor, cohortID string) error {
	const msg = "Não foi possível validar a turma. Tente novamente em instantes."
	if author.HasRole(actor.RoleAdmin) {
		if cohortID == "" {
			return nil
		}
		exists, err := s.academic.CohortExists(ctx, cohortID)
		if err != nil {
			return apperr.Internal(msg, fmt.Errorf("cohort exists: %w", err))
		}
		if !exists {
			return apperr.NotFound(msgCohortNotFound)
		}
		return nil
	}
	set, err := s.advisedCohorts(ctx, author)
	if err != nil {
		return apperr.Internal(msg, err)
	}
	if cohortID == "" || !set[cohortID] {
		return apperr.Forbidden(msgAdvisorCohort)
	}
	return nil
}

// Exclusão
func (s *Service) Delete(ctx context.Context, who actor.Actor, id string) error {
	const action = "aviso.excluido"
	a, found, err := s.store.Get(ctx, id)
	if err != nil {
		return apperr.Internal("Não foi possível carregar o aviso. Tente novamente em instantes.", fmt.Errorf("get announcement: %w", err))
	}
	if !found {
		return apperr.NotFound(msgNotFound)
	}
	if !a.canRemove(who) {
		audit.Record(ctx, audit.Event{Action: action, Result: audit.Failure, Reason: msgRemoveOwn, Resource: resource(id)})
		return apperr.Forbidden(msgRemoveOwn)
	}
	if err := s.store.Delete(ctx, id); errors.Is(err, errNotFound) {
		return apperr.NotFound(msgNotFound)
	} else if err != nil {
		return apperr.Internal("Não foi possível excluir o aviso. Tente novamente em instantes.", fmt.Errorf("delete announcement: %w", err))
	}
	audit.Record(ctx, audit.Event{Action: action, Result: audit.Success, Resource: resource(id)})
	return nil
}
