package deliveries

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/parallel"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/storage"
)

type Service struct {
	store    Store
	projects Projects
	files    Files
	newID    func() string
	now      func() time.Time
}

const (
	uploadTTL   = 15 * time.Minute
	downloadTTL = 5 * time.Minute
)

func NewService(store Store, projects Projects, files Files) *Service {
	return &Service{store: store, projects: projects, files: files, newID: uuid.NewString, now: time.Now}
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

// SubmissionsOfProgram feeds the staff panel's status grid in one call; an
// advisor gets only the projects they advise.
func (s *Service) SubmissionsOfProgram(ctx context.Context, viewer actor.Actor, programID string) ([]Submission, error) {
	projects, err := s.projects.ProjectsOfProgram(ctx, viewer, programID)
	if err != nil {
		return nil, err
	}
	perProject, err := parallel.Map(projects, func(p groups.Project) ([]Submission, error) {
		return s.store.SubmissionsOfProject(ctx, p.ID)
	})
	if err != nil {
		return nil, apperr.Internal("falha ao listar entregas", err)
	}
	return slices.Concat(perProject...), nil
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
	previous, _, err := s.store.GetSubmission(ctx, p.ID, a.ID)
	if err != nil {
		return Submission{}, apperr.Internal("falha ao carregar a entrega anterior", err)
	}
	if err := s.checkFiles(ctx, p.ID, a, answers, previous.Answers); err != nil {
		return Submission{}, err
	}
	sub := Submission{ActivityID: a.ID, ProjectID: p.ID, SubmittedAt: s.timestamp(), SubmittedBy: member.Name, Answers: answers}
	if err := s.store.PutSubmission(ctx, sub); err != nil {
		return Submission{}, apperr.Internal("falha ao salvar entrega", err)
	}
	s.deleteFiles(ctx, replaced(previous.Answers, answers))
	audit.Record(ctx, audit.Event{Action: "entrega.enviada", Result: audit.Success, Resource: activityResource(a.ID),
		Details: map[string]any{"projetoId": p.ID}})
	return sub, nil
}

// RemoveSubmission is how the academic team hands an activity back to the
// group; nobody else edits a submission's content.
func (s *Service) RemoveSubmission(ctx context.Context, reviewer actor.Actor, projectID, activityID string) error {
	p, err := s.project(ctx, projectID)
	if err != nil {
		return err
	}
	if !groups.CanReview(reviewer, p) {
		audit.Record(ctx, audit.Event{Action: "entrega.removida", Result: audit.Failure, Reason: msgReviewersOnly,
			Resource: activityResource(activityID), Details: map[string]any{"projetoId": projectID}})
		return apperr.Forbidden(msgReviewersOnly)
	}
	removed, err := s.store.DeleteSubmission(ctx, projectID, activityID)
	if errors.Is(err, errNotFound) {
		return apperr.NotFound(msgSubmissionNotFound)
	}
	if err != nil {
		return apperr.Internal("falha ao remover entrega", err)
	}
	s.deleteFiles(ctx, replaced(removed.Answers, nil))
	audit.Record(ctx, audit.Event{Action: "entrega.removida", Result: audit.Success, Resource: activityResource(activityID),
		Details: map[string]any{"projetoId": projectID}})
	return nil
}

// RequestUpload lets a member send one file for a file field. The answer
// only changes when the group submits the form with the returned key.
func (s *Service) RequestUpload(ctx context.Context, member actor.Actor, projectID, activityID string, in UploadRequest) (UploadTicket, error) {
	if s.files == nil {
		return UploadTicket{}, apperr.Unavailable(msgNoStorage, nil)
	}
	p, err := s.project(ctx, projectID)
	if err != nil {
		return UploadTicket{}, err
	}
	if !groups.IsMember(member, p) {
		return UploadTicket{}, apperr.Forbidden(msgMembersOnly)
	}
	a, found, err := s.store.GetActivity(ctx, activityID)
	if err != nil {
		return UploadTicket{}, apperr.Internal("falha ao carregar atividade", err)
	}
	if !found {
		return UploadTicket{}, apperr.NotFound(msgActivityNotFound)
	}
	i := slices.IndexFunc(a.Fields, func(f Field) bool { return f.ID == in.FieldID })
	switch {
	case i < 0:
		return UploadTicket{}, apperr.NotFound(msgFieldNotFound)
	case a.Fields[i].Type != FieldFile:
		return UploadTicket{}, apperr.Invalid(msgNotAFile)
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
	key := fieldPrefix(p.ID, a.ID, in.FieldID) + s.newID() + "/" + name
	up, err := s.files.PresignUpload(ctx, key, kind, storage.MaxBytes, uploadTTL)
	if err != nil {
		return UploadTicket{}, apperr.Internal("falha ao preparar o envio do arquivo", err)
	}
	return UploadTicket{URL: up.URL, Fields: up.Fields, Key: key, ExpiresAt: up.ExpiresAt.UTC().Format(time.RFC3339), MaxBytes: storage.MaxBytes}, nil
}

// checkFiles accepts, in a file field, the answer already submitted or a
// file this group uploaded for this field. Without storage, any name goes,
// as before.
func (s *Service) checkFiles(ctx context.Context, projectID string, a Activity, answers, previous map[string]string) error {
	if s.files == nil {
		return nil
	}
	for _, f := range a.Fields {
		v, ok := answers[f.ID]
		if f.Type != FieldFile || !ok || v == previous[f.ID] {
			continue
		}
		if !strings.HasPrefix(v, fieldPrefix(projectID, a.ID, f.ID)) {
			return apperr.Invalid(fmt.Sprintf("envie o arquivo do campo %q pelo botão de envio", f.Label))
		}
		size, found, err := s.files.Size(ctx, v)
		if err != nil {
			return apperr.Internal("falha ao conferir o arquivo enviado", err)
		}
		if !found || size > storage.MaxBytes {
			return apperr.Invalid(fmt.Sprintf("o arquivo do campo %q não chegou — envie de novo", f.Label))
		}
	}
	return nil
}

// replaced lists the stored files that before had and after no longer has.
func replaced(before, after map[string]string) []string {
	var keys []string
	for id, v := range before {
		if isStoredFile(v) && after[id] != v {
			keys = append(keys, v)
		}
	}
	return keys
}

// deleteFiles is best effort: a file left behind costs little and must not
// undo the submission.
func (s *Service) deleteFiles(ctx context.Context, keys []string) {
	if s.files == nil {
		return
	}
	for _, k := range keys {
		if err := s.files.Delete(ctx, k); err != nil {
			slog.WarnContext(ctx, "file not deleted", "key", k, "error", err)
		}
	}
}

func (s *Service) DownloadFile(ctx context.Context, viewer actor.Actor, projectID, activityID, fieldID string) (Download, error) {
	if s.files == nil {
		return Download{}, apperr.Unavailable(msgNoStorage, nil)
	}
	p, err := s.project(ctx, projectID)
	if err != nil {
		return Download{}, err
	}
	if !groups.CanView(viewer, p) {
		return Download{}, apperr.Forbidden(msgMembersAndStaff)
	}
	sub, found, err := s.store.GetSubmission(ctx, p.ID, activityID)
	if err != nil {
		return Download{}, apperr.Internal("falha ao carregar a entrega", err)
	}
	key := sub.Answers[fieldID]
	if !found || !isStoredFile(key) {
		return Download{}, apperr.NotFound(msgNoFileHere)
	}
	name := fileNameOf(key)
	url, err := s.files.PresignDownload(ctx, key, name, downloadTTL)
	if err != nil {
		return Download{}, apperr.Internal("falha ao preparar o download", err)
	}
	return Download{URL: url, Name: name}, nil
}
