package references

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

type Service struct {
	store    Store
	works    Works
	projects Projects
	now      func() time.Time
}

func NewService(store Store, works Works, projects Projects) *Service {
	return &Service{store: store, works: works, projects: projects, now: time.Now}
}

func (s *Service) Search(ctx context.Context, term, page string) (SearchResult, error) {
	term = strings.TrimSpace(term)
	if n := utf8.RuneCountInString(term); n < minSearchLen || n > maxSearchLen {
		return SearchResult{}, apperr.Invalid(msgSearchLength)
	}
	p := 1
	if page != "" {
		n, err := strconv.Atoi(page)
		if err != nil || n < 1 || n > maxPage {
			return SearchResult{}, apperr.Invalid(msgInvalidPage)
		}
		p = n
	}
	total, works, err := s.works.Search(ctx, term, p)
	if err != nil {
		return SearchResult{}, apperr.Unavailable(msgSearchUnavailable, err)
	}
	result := SearchResult{Total: total, Page: p, Works: make([]FoundWork, 0, len(works))}
	for _, w := range works {
		result.Works = append(result.Works, found(w))
	}
	return result, nil
}

func (s *Service) workByDOI(ctx context.Context, doi string) (Work, error) {
	w, err := s.works.WorkByDOI(ctx, doi)
	if errors.Is(err, errWorkNotFound) {
		return Work{}, apperr.NotFound(msgDOINotFound)
	}
	if err != nil {
		return Work{}, apperr.Unavailable(msgDOIUnavailable, err)
	}
	return w, nil
}

func (s *Service) LookupDOI(ctx context.Context, raw string) (FormattedWork, error) {
	doi, ok := normalizeDOI(raw)
	if !ok {
		return FormattedWork{}, apperr.Invalid(msgInvalidDOI)
	}
	w, err := s.workByDOI(ctx, doi)
	if err != nil {
		return FormattedWork{}, err
	}
	text, html := formatABNT(w, s.now())
	return FormattedWork{FoundWork: found(w), ABNT: text, ABNTHTML: html}, nil
}

func (s *Service) project(ctx context.Context, id string) (groups.Project, error) {
	p, ok, err := s.projects.Project(ctx, id)
	if err != nil {
		return groups.Project{}, apperr.Internal("Não foi possível carregar o projeto. Tente novamente em instantes.", err)
	}
	if !ok {
		return groups.Project{}, apperr.NotFound(msgProjectNotFound)
	}
	return p, nil
}

// ReferencesOf sorts alphabetically, the order of a reference list under
// NBR 6023 (author-date system).
func (s *Service) ReferencesOf(ctx context.Context, viewer actor.Actor, projectID string) ([]Reference, error) {
	p, err := s.project(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if !groups.CanView(viewer, p) {
		return nil, apperr.Forbidden(msgMembersAndStaff)
	}
	refs, err := s.store.ReferencesOf(ctx, p.ID)
	if err != nil {
		return nil, apperr.Internal("Não foi possível carregar as referências. Tente novamente em instantes.", err)
	}
	slices.SortStableFunc(refs, func(a, b Reference) int {
		return strings.Compare(strings.ToUpper(a.ABNT), strings.ToUpper(b.ABNT))
	})
	return refs, nil
}

// requireMember: the list belongs to the group, so only members change it.
func (s *Service) requireMember(ctx context.Context, member actor.Actor, p groups.Project, action string) error {
	if groups.IsMember(member, p) {
		return nil
	}
	audit.Record(ctx, audit.Event{Action: action, Result: audit.Failure, Reason: msgMembersOnly,
		Resource: &audit.Resource{Type: "projeto", ID: p.ID}})
	return apperr.Forbidden(msgMembersOnly)
}

func (s *Service) Add(ctx context.Context, member actor.Actor, projectID string, in ReferenceInput) (Reference, error) {
	doi, ok := normalizeDOI(in.DOI)
	if !ok {
		return Reference{}, apperr.Invalid(msgInvalidDOI)
	}
	p, err := s.project(ctx, projectID)
	if err != nil {
		return Reference{}, err
	}
	if err := s.requireMember(ctx, member, p, "referencia.adicionada"); err != nil {
		return Reference{}, err
	}
	w, err := s.workByDOI(ctx, doi)
	if err != nil {
		return Reference{}, err
	}

	now := s.now()
	text, html := formatABNT(w, now)
	// Crossref's canonical DOI, not the one the student typed.
	canonical := orDefault(w.DOI, doi)
	ref := Reference{
		ID:        referenceID(canonical),
		ProjectID: p.ID,
		DOI:       canonical,
		Title:     fullTitle(w),
		Year:      w.Year,
		ABNT:      text,
		ABNTHTML:  html,
		AddedBy:   member.Name,
		AddedAt:   now.UTC().Format(time.RFC3339),
	}
	err = s.store.PutReference(ctx, ref, member.Sub)
	if errors.Is(err, errDuplicate) {
		return Reference{}, apperr.Conflict(msgDuplicate)
	}
	if err != nil {
		return Reference{}, apperr.Internal("Não foi possível salvar a referência. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: "referencia.adicionada", Result: audit.Success,
		Resource: &audit.Resource{Type: "referencia", ID: ref.ID}, Details: map[string]any{"projetoId": p.ID, "doi": ref.DOI}})
	return ref, nil
}

func (s *Service) Remove(ctx context.Context, member actor.Actor, projectID, id string) error {
	p, err := s.project(ctx, projectID)
	if err != nil {
		return err
	}
	if err := s.requireMember(ctx, member, p, "referencia.removida"); err != nil {
		return err
	}
	switch err := s.store.DeleteReference(ctx, p.ID, id); {
	case errors.Is(err, errNotFound):
		return apperr.NotFound(msgReferenceNotFound)
	case err != nil:
		return apperr.Internal("Não foi possível remover a referência. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: "referencia.removida", Result: audit.Success,
		Resource: &audit.Resource{Type: "referencia", ID: id}, Details: map[string]any{"projetoId": p.ID}})
	return nil
}
