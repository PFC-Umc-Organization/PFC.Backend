package deliveries

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

type Service struct {
	store    Store
	projects Projects
	newID    func() string
	now      func() time.Time
}

func NewService(store Store, projects Projects) *Service {
	return &Service{store: store, projects: projects, newID: uuid.NewString, now: time.Now}
}

func activityResource(id string) *audit.Resource { return &audit.Resource{Type: "atividade", ID: id} }

func (s *Service) timestamp() string { return s.now().UTC().Format(time.RFC3339) }

func (s *Service) ListActivities(ctx context.Context) ([]Activity, error) {
	activities, err := s.store.ListActivities(ctx)
	if err != nil {
		return nil, apperr.Internal("falha ao listar atividades", err)
	}
	slices.SortStableFunc(activities, func(a, b Activity) int { return strings.Compare(a.Deadline, b.Deadline) })
	return activities, nil
}

func (s *Service) CreateActivity(ctx context.Context, in ActivityInput) (Activity, error) {
	in = in.normalized()
	if msg := in.validate(); msg != "" {
		return Activity{}, apperr.Invalid(msg)
	}
	inputs, msg := normalizedFields(in.Fields)
	if msg != "" {
		return Activity{}, apperr.Invalid(msg)
	}
	a := Activity{
		ID:          s.newID(),
		Title:       in.Title,
		Description: in.Description,
		Deadline:    in.Deadline,
		PublishedAt: s.timestamp(),
		Fields:      make([]Field, 0, len(inputs)),
	}
	for _, f := range inputs {
		a.Fields = append(a.Fields, Field{ID: s.newID(), Label: f.Label, Type: f.Type, Required: f.Required})
	}
	if err := s.store.CreateActivity(ctx, a); err != nil {
		return Activity{}, apperr.Internal("falha ao criar atividade", err)
	}
	audit.Record(ctx, audit.Event{Action: "atividade.criada", Result: audit.Success, Resource: activityResource(a.ID),
		Details: map[string]any{"titulo": a.Title, "prazo": a.Deadline}})
	return a, nil
}

func (s *Service) UpdateActivity(ctx context.Context, id string, in ActivityInput) (Activity, error) {
	in = in.normalized()
	if msg := in.validate(); msg != "" {
		return Activity{}, apperr.Invalid(msg)
	}
	a, err := s.store.UpdateActivity(ctx, id, in)
	if errors.Is(err, errNotFound) {
		return Activity{}, apperr.NotFound(msgActivityNotFound)
	}
	if err != nil {
		return Activity{}, apperr.Internal("falha ao atualizar atividade", err)
	}
	audit.Record(ctx, audit.Event{Action: "atividade.atualizada", Result: audit.Success, Resource: activityResource(id),
		Details: map[string]any{"titulo": in.Title, "prazo": in.Deadline}})
	return a, nil
}

func (s *Service) DeleteActivity(ctx context.Context, id string) error {
	err := s.store.DeleteActivity(ctx, id)
	if errors.Is(err, errNotFound) {
		return apperr.NotFound(msgActivityNotFound)
	}
	if err != nil {
		return apperr.Internal("falha ao remover atividade", err)
	}
	audit.Record(ctx, audit.Event{Action: "atividade.removida", Result: audit.Success, Resource: activityResource(id)})
	return nil
}

func (s *Service) AddField(ctx context.Context, activityID string, in FieldInput) (Field, error) {
	in, msg := in.normalized()
	if msg != "" {
		return Field{}, apperr.Invalid(msg)
	}
	f := Field{ID: s.newID(), Label: in.Label, Type: in.Type, Required: in.Required}
	switch err := s.store.AddField(ctx, activityID, f); {
	case errors.Is(err, errNotFound):
		return Field{}, apperr.NotFound(msgActivityNotFound)
	case errors.Is(err, errFull):
		return Field{}, apperr.Conflict(msgFieldLimit)
	case err != nil:
		return Field{}, apperr.Internal("falha ao adicionar campo", err)
	}
	audit.Record(ctx, audit.Event{Action: "atividade.campo_adicionado", Result: audit.Success, Resource: activityResource(activityID),
		Details: map[string]any{"campoId": f.ID, "rotulo": f.Label, "tipo": string(f.Type)}})
	return f, nil
}

// RemoveField refuses the last field: every activity needs at least one.
func (s *Service) RemoveField(ctx context.Context, activityID, fieldID string) error {
	a, found, err := s.store.GetActivity(ctx, activityID)
	if err != nil {
		return apperr.Internal("falha ao remover campo", err)
	}
	if !found {
		return apperr.NotFound(msgActivityNotFound)
	}
	position := slices.IndexFunc(a.Fields, func(f Field) bool { return f.ID == fieldID })
	if position < 0 {
		return apperr.NotFound(msgFieldNotFound)
	}
	if len(a.Fields) <= 1 {
		return apperr.Conflict(msgLastField)
	}
	switch err := s.store.RemoveField(ctx, activityID, position, fieldID); {
	case errors.Is(err, errStale):
		return apperr.NotFound(msgFieldNotFound)
	case err != nil:
		return apperr.Internal("falha ao remover campo", err)
	}
	audit.Record(ctx, audit.Event{Action: "atividade.campo_removido", Result: audit.Success, Resource: activityResource(activityID),
		Details: map[string]any{"campoId": fieldID}})
	return nil
}

func (s *Service) SubmissionsOfActivity(ctx context.Context, activityID string) ([]Submission, error) {
	submissions, err := s.store.SubmissionsOfActivity(ctx, activityID)
	if err != nil {
		return nil, apperr.Internal("falha ao listar entregas", err)
	}
	return submissions, nil
}

func (s *Service) project(ctx context.Context, id string) (groups.Project, error) {
	p, found, err := s.projects.Project(ctx, id)
	if err != nil {
		return groups.Project{}, apperr.Internal("falha ao carregar projeto", err)
	}
	if !found {
		return groups.Project{}, apperr.NotFound(msgProjectNotFound)
	}
	return p, nil
}

func (s *Service) SubmissionsOfProject(ctx context.Context, viewer actor.Actor, projectID string) ([]Submission, error) {
	p, err := s.project(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if !groups.CanView(viewer, p) {
		return nil, apperr.Forbidden(msgMembersAndStaff)
	}
	submissions, err := s.store.SubmissionsOfProject(ctx, p.ID)
	if err != nil {
		return nil, apperr.Internal("falha ao listar entregas", err)
	}
	return submissions, nil
}

// Submit overwrites the group's previous submission for the activity.
func (s *Service) Submit(ctx context.Context, member actor.Actor, projectID, activityID string, in SubmissionInput) (Submission, error) {
	p, err := s.project(ctx, projectID)
	if err != nil {
		return Submission{}, err
	}
	if !groups.IsMember(member, p) {
		audit.Record(ctx, audit.Event{Action: "entrega.enviada", Result: audit.Failure, Reason: msgMembersOnly,
			Resource: &audit.Resource{Type: "projeto", ID: p.ID}})
		return Submission{}, apperr.Forbidden(msgMembersOnly)
	}
	a, found, err := s.store.GetActivity(ctx, activityID)
	if err != nil {
		return Submission{}, apperr.Internal("falha ao carregar atividade", err)
	}
	if !found {
		return Submission{}, apperr.NotFound(msgActivityNotFound)
	}
	answers, msg := validAnswers(a.Fields, in.Answers)
	if msg != "" {
		return Submission{}, apperr.Invalid(msg)
	}
	sub := Submission{ActivityID: a.ID, ProjectID: p.ID, SubmittedAt: s.timestamp(), SubmittedBy: member.Name, Answers: answers}
	if err := s.store.PutSubmission(ctx, sub); err != nil {
		return Submission{}, apperr.Internal("falha ao salvar entrega", err)
	}
	audit.Record(ctx, audit.Event{Action: "entrega.enviada", Result: audit.Success, Resource: activityResource(a.ID),
		Details: map[string]any{"projetoId": p.ID}})
	return sub, nil
}

// RemoveSubmission is how the academic team hands an activity back to the
// group; nobody else edits a submission's content.
func (s *Service) RemoveSubmission(ctx context.Context, projectID, activityID string) error {
	err := s.store.DeleteSubmission(ctx, projectID, activityID)
	if errors.Is(err, errNotFound) {
		return apperr.NotFound(msgSubmissionNotFound)
	}
	if err != nil {
		return apperr.Internal("falha ao remover entrega", err)
	}
	audit.Record(ctx, audit.Event{Action: "entrega.removida", Result: audit.Success, Resource: activityResource(activityID),
		Details: map[string]any{"projetoId": projectID}})
	return nil
}
