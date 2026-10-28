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
		return nil, apperr.Internal("Não foi possível carregar as atividades. Tente novamente em instantes.", err)
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
		return Activity{}, apperr.Internal("Não foi possível criar a atividade. Tente novamente em instantes.", err)
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
		return Activity{}, apperr.Internal("Não foi possível atualizar a atividade. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: "atividade.atualizada", Result: audit.Success, Resource: activityResource(id),
		Details: map[string]any{"titulo": in.Title, "prazo": in.Deadline}})
	return a, nil
}

func (s *Service) DeleteActivity(ctx context.Context, id string) error {
	projects, err := s.store.DeleteActivity(ctx, id)
	if errors.Is(err, errNotFound) {
		return apperr.NotFound(msgActivityNotFound)
	}
	if err != nil {
		return apperr.Internal("Não foi possível excluir a atividade. Tente novamente em instantes.", err)
	}
	for _, p := range projects {
		s.deleteFolder(ctx, filesPrefix+p+"/"+id+"/")
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
		return Field{}, apperr.Internal("Não foi possível adicionar o campo. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: "atividade.campo_adicionado", Result: audit.Success, Resource: activityResource(activityID),
		Details: map[string]any{"campoId": f.ID, "rotulo": f.Label, "tipo": string(f.Type)}})
	return f, nil
}

// RemoveField refuses the last field: every activity needs at least one.
func (s *Service) RemoveField(ctx context.Context, activityID, fieldID string) error {
	a, found, err := s.store.GetActivity(ctx, activityID)
	if err != nil {
		return apperr.Internal("Não foi possível remover o campo. Tente novamente em instantes.", err)
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
		return apperr.Internal("Não foi possível remover o campo. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: "atividade.campo_removido", Result: audit.Success, Resource: activityResource(activityID),
		Details: map[string]any{"campoId": fieldID}})
	return nil
}

func (s *Service) SubmissionsOfActivity(ctx context.Context, activityID string) ([]Submission, error) {
	submissions, err := s.store.SubmissionsOfActivity(ctx, activityID)
	if err != nil {
		return nil, apperr.Internal("Não foi possível carregar as entregas. Tente novamente em instantes.", err)
	}
	return submissions, nil
}

func (s *Service) project(ctx context.Context, id string) (groups.Project, error) {
	p, found, err := s.projects.Project(ctx, id)
	if err != nil {
		return groups.Project{}, apperr.Internal("Não foi possível carregar o projeto. Tente novamente em instantes.", err)
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
		return nil, apperr.Internal("Não foi possível carregar as entregas. Tente novamente em instantes.", err)
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
		return nil, apperr.Internal("Não foi possível carregar as entregas. Tente novamente em instantes.", err)
	}
	return append([]Submission{}, slices.Concat(perProject...)...), nil
}

// Submit creates or edits the group's submission. After the deadline only a
// group that submitted in time can still edit; the first submission's time
// is kept.
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
	if err := s.failed(ctx, p.ID, activityID); err != nil {
		return Submission{}, err
	}
	a, found, err := s.store.GetActivity(ctx, activityID)
	if err != nil {
		return Submission{}, apperr.Internal("Não foi possível carregar a atividade. Tente novamente em instantes.", err)
	}
	if !found {
		return Submission{}, apperr.NotFound(msgActivityNotFound)
	}
	answers, msg := validAnswers(a.Fields, in.Answers)
	if msg != "" {
		return Submission{}, apperr.Invalid(msg)
	}
	previous, found, err := s.store.GetSubmission(ctx, p.ID, a.ID)
	if err != nil {
		return Submission{}, apperr.Internal("Não foi possível carregar a entrega anterior. Tente novamente em instantes.", err)
	}
	closed := a.closed(s.now())
	if closed && !found {
		return Submission{}, s.deadlinePassed(ctx, p.ID, a.ID)
	}
	if err := s.checkFiles(ctx, p.ID, a, answers, previous.Answers); err != nil {
		return Submission{}, err
	}
	sub := Submission{ActivityID: a.ID, ProjectID: p.ID, SubmittedAt: s.timestamp(), SubmittedBy: member.Name, Answers: answers}
	if found && previous.SubmittedAt != "" {
		sub.SubmittedAt, sub.SubmittedBy = previous.SubmittedAt, previous.SubmittedBy
		sub.UpdatedAt, sub.UpdatedBy = s.timestamp(), member.Name
	}
	switch err := s.store.PutSubmission(ctx, sub, closed); {
	case errors.Is(err, errNotFound):
		return Submission{}, s.deadlinePassed(ctx, p.ID, a.ID)
	case err != nil:
		return Submission{}, apperr.Internal("Não foi possível salvar a entrega. Tente novamente em instantes.", err)
	}
	s.deleteFiles(ctx, replaced(previous.Answers, answers))
	audit.Record(ctx, audit.Event{Action: "entrega.enviada", Result: audit.Success, Resource: activityResource(a.ID),
		Details: map[string]any{"projetoId": p.ID}})
	return sub, nil
}

func (s *Service) deadlinePassed(ctx context.Context, projectID, activityID string) error {
	audit.Record(ctx, audit.Event{Action: "entrega.enviada", Result: audit.Failure, Reason: msgDeadlinePassed,
		Resource: activityResource(activityID), Details: map[string]any{"projetoId": projectID}})
	return apperr.Conflict(msgDeadlinePassed)
}

// RemoveSubmission is how the academic team hands an activity back to the
// group; nobody else edits a submission's content. After the deadline the
// group can no longer submit it again.
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
		return apperr.Internal("Não foi possível remover a entrega. Tente novamente em instantes.", err)
	}
	s.deleteFiles(ctx, replaced(removed.Answers, nil))
	audit.Record(ctx, audit.Event{Action: "entrega.removida", Result: audit.Success, Resource: activityResource(activityID),
		Details: map[string]any{"projetoId": projectID}})
	return nil
}

// Validate records the advisor's verdict on the submission as it is now:
// only the project's advisor gives it; the coordination and the group read
// it. Editing the submission afterwards drops it.
func (s *Service) Validate(ctx context.Context, advisor actor.Actor, projectID, activityID string, in ValidationInput) (Submission, error) {
	action := "entrega.validada"
	if in.Status == NotValidated {
		action = "entrega.nao_validada"
	}
	p, err := s.project(ctx, projectID)
	if err != nil {
		return Submission{}, err
	}
	if !groups.Advises(advisor, p) {
		audit.Record(ctx, audit.Event{Action: action, Result: audit.Failure, Reason: msgAdvisorValidates,
			Resource: activityResource(activityID), Details: map[string]any{"projetoId": p.ID}})
		return Submission{}, apperr.Forbidden(msgAdvisorValidates)
	}
	in, msg := in.normalized()
	if msg != "" {
		return Submission{}, apperr.Invalid(msg)
	}
	v := Validation{Status: in.Status, Comment: in.Comment, By: advisor.Sub, ByName: advisor.Name, At: s.timestamp()}
	sub, err := s.store.SetValidation(ctx, p.ID, activityID, v)
	if errors.Is(err, errNotFound) {
		return Submission{}, apperr.NotFound(msgSubmissionNotFound)
	}
	if err != nil {
		return Submission{}, apperr.Internal("Não foi possível registrar a validação. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: action, Result: audit.Success, Resource: activityResource(activityID),
		Details: map[string]any{"projetoId": p.ID}})
	return sub, nil
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
	if err := s.failed(ctx, p.ID, activityID); err != nil {
		return UploadTicket{}, err
	}
	a, found, err := s.store.GetActivity(ctx, activityID)
	if err != nil {
		return UploadTicket{}, apperr.Internal("Não foi possível carregar a atividade. Tente novamente em instantes.", err)
	}
	if !found {
		return UploadTicket{}, apperr.NotFound(msgActivityNotFound)
	}
	if a.closed(s.now()) {
		_, submitted, err := s.store.GetSubmission(ctx, p.ID, a.ID)
		if err != nil {
			return UploadTicket{}, apperr.Internal("Não foi possível carregar a entrega. Tente novamente em instantes.", err)
		}
		if !submitted {
			return UploadTicket{}, apperr.Conflict(msgDeadlinePassed)
		}
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
		return UploadTicket{}, apperr.Internal("Não foi possível preparar o envio do arquivo. Tente novamente em instantes.", err)
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
			return apperr.Invalid(fmt.Sprintf("Use o botão Escolher arquivo para enviar o arquivo do campo %q.", f.Label))
		}
		size, found, err := s.files.Size(ctx, v)
		if err != nil {
			return apperr.Internal("Não foi possível conferir o arquivo enviado. Tente novamente em instantes.", err)
		}
		if !found || size > storage.MaxBytes {
			return apperr.Invalid(fmt.Sprintf("O arquivo do campo %q não foi recebido. Envie-o novamente.", f.Label))
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

// ProjectDeleted removes the project's files, abandoned uploads included.
func (s *Service) ProjectDeleted(ctx context.Context, projectID string) {
	s.deleteFolder(ctx, filesPrefix+projectID+"/")
}

func (s *Service) deleteFolder(ctx context.Context, prefix string) {
	if s.files == nil {
		return
	}
	if err := s.files.DeletePrefix(ctx, prefix); err != nil {
		slog.WarnContext(ctx, "files not deleted", "prefix", prefix, "error", err)
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
		return Download{}, apperr.Internal("Não foi possível carregar a entrega. Tente novamente em instantes.", err)
	}
	key := sub.Answers[fieldID]
	if !found || !isStoredFile(key) {
		return Download{}, apperr.NotFound(msgNoFileHere)
	}
	name := fileNameOf(key)
	url, err := s.files.PresignDownload(ctx, key, name, downloadTTL)
	if err != nil {
		return Download{}, apperr.Internal("Não foi possível preparar o download. Tente novamente em instantes.", err)
	}
	return Download{URL: url, Name: name}, nil
}

// groupPenalties reads what the penalties are computed from, at once.
func (s *Service) groupPenalties(ctx context.Context, projectID string) (Penalties, error) {
	var activities []Activity
	var activitiesErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		activities, activitiesErr = s.store.ListActivities(ctx)
	}()
	p, err := s.projectPenalties(ctx, projectID)
	<-done
	if err := errors.Join(activitiesErr, err); err != nil {
		return Penalties{}, err
	}
	return penaltiesOf(projectID, activities, p.submissions, p.records, s.now()), nil
}

type penaltySources struct {
	submissions []Submission
	records     []penaltyRecord
}

func (s *Service) projectPenalties(ctx context.Context, projectID string) (penaltySources, error) {
	var out penaltySources
	var submissionsErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		out.submissions, submissionsErr = s.store.SubmissionsOfProject(ctx, projectID)
	}()
	records, err := s.store.PenaltiesOfProject(ctx, projectID)
	<-done
	out.records = records
	return out, errors.Join(submissionsErr, err)
}

// failed refuses a member of a group that reached MaxPenalties.
func (s *Service) failed(ctx context.Context, projectID, activityID string) error {
	penalties, err := s.groupPenalties(ctx, projectID)
	if err != nil {
		return apperr.Internal("Não foi possível carregar as penalidades. Tente novamente em instantes.", err)
	}
	if !penalties.Failed {
		return nil
	}
	audit.Record(ctx, audit.Event{Action: "entrega.enviada", Result: audit.Failure, Reason: msgGroupFailed,
		Resource: activityResource(activityID), Details: map[string]any{"projetoId": projectID}})
	return apperr.Conflict(msgGroupFailed)
}

func (s *Service) PenaltiesOfProject(ctx context.Context, viewer actor.Actor, projectID string) (Penalties, error) {
	p, err := s.project(ctx, projectID)
	if err != nil {
		return Penalties{}, err
	}
	if !groups.CanView(viewer, p) {
		return Penalties{}, apperr.Forbidden(msgMembersAndStaff)
	}
	penalties, err := s.groupPenalties(ctx, p.ID)
	if err != nil {
		return Penalties{}, apperr.Internal("Não foi possível carregar as penalidades. Tente novamente em instantes.", err)
	}
	return penalties, nil
}

// PenaltiesOfProgram feeds the staff panel; an advisor gets only the
// projects they advise.
func (s *Service) PenaltiesOfProgram(ctx context.Context, viewer actor.Actor, programID string) ([]Penalties, error) {
	projects, err := s.projects.ProjectsOfProgram(ctx, viewer, programID)
	if err != nil {
		return nil, err
	}
	activities, err := s.store.ListActivities(ctx)
	if err != nil {
		return nil, apperr.Internal("Não foi possível carregar as penalidades. Tente novamente em instantes.", err)
	}
	out, err := parallel.Map(projects, func(p groups.Project) (Penalties, error) {
		src, err := s.projectPenalties(ctx, p.ID)
		return penaltiesOf(p.ID, activities, src.submissions, src.records, s.now()), err
	})
	if err != nil {
		return nil, apperr.Internal("Não foi possível carregar as penalidades. Tente novamente em instantes.", err)
	}
	return append([]Penalties{}, out...), nil
}

func (s *Service) reviewedProject(ctx context.Context, reviewer actor.Actor, projectID, action string) (groups.Project, error) {
	p, err := s.project(ctx, projectID)
	if err != nil {
		return groups.Project{}, err
	}
	if !groups.CanReview(reviewer, p) {
		audit.Record(ctx, audit.Event{Action: action, Result: audit.Failure, Reason: msgPenaltyReviewers,
			Resource: &audit.Resource{Type: "projeto", ID: p.ID}})
		return groups.Project{}, apperr.Forbidden(msgPenaltyReviewers)
	}
	return p, nil
}

// ApplyPenalty adds a penalty for a reason other than a missed deadline,
// optionally about one activity.
func (s *Service) ApplyPenalty(ctx context.Context, reviewer actor.Actor, projectID string, in PenaltyInput) (Penalties, error) {
	p, err := s.reviewedProject(ctx, reviewer, projectID, "penalidade.aplicada")
	if err != nil {
		return Penalties{}, err
	}
	reason, msg := validReason(in.Reason)
	if msg != "" {
		return Penalties{}, apperr.Invalid(msg)
	}
	activityID := strings.TrimSpace(in.ActivityID)
	if activityID != "" {
		_, found, err := s.store.GetActivity(ctx, activityID)
		if err != nil {
			return Penalties{}, apperr.Internal("Não foi possível carregar a atividade. Tente novamente em instantes.", err)
		}
		if !found {
			return Penalties{}, apperr.NotFound(msgActivityNotFound)
		}
	}
	r := penaltyRecord{ID: manualPenaltyPrefix + s.newID(), ActivityID: activityID, Reason: reason, AppliedAt: s.timestamp(), AppliedBy: reviewer.Name}
	if err := s.store.AddPenalty(ctx, p.ID, r); err != nil {
		return Penalties{}, apperr.Internal("Não foi possível aplicar a penalidade. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: "penalidade.aplicada", Result: audit.Success, Resource: &audit.Resource{Type: "projeto", ID: p.ID},
		Details: map[string]any{"penalidadeId": r.ID, "atividadeId": activityID}})
	return s.PenaltiesOfProject(ctx, reviewer, p.ID)
}

// WaivePenalty takes back one active penalty, from a deadline or manual; a
// failed group with fewer than MaxPenalties left can submit again.
func (s *Service) WaivePenalty(ctx context.Context, reviewer actor.Actor, projectID, penaltyID string, in WaiverInput) (Penalties, error) {
	p, err := s.reviewedProject(ctx, reviewer, projectID, "penalidade.perdoada")
	if err != nil {
		return Penalties{}, err
	}
	reason, msg := validReason(in.Reason)
	if msg != "" {
		return Penalties{}, apperr.Invalid(msg)
	}
	current, err := s.groupPenalties(ctx, p.ID)
	if err != nil {
		return Penalties{}, apperr.Internal("Não foi possível carregar as penalidades. Tente novamente em instantes.", err)
	}
	i := slices.IndexFunc(current.Items, func(x Penalty) bool { return x.ID == penaltyID })
	switch {
	case i < 0:
		return Penalties{}, apperr.NotFound(msgPenaltyNotFound)
	case current.Items[i].Waiver != nil:
		return Penalties{}, apperr.Conflict(msgAlreadyWaived)
	}
	switch err := s.store.WaivePenalty(ctx, p.ID, penaltyID, Waiver{Reason: reason, By: reviewer.Name, At: s.timestamp()}); {
	case errors.Is(err, errStale):
		return Penalties{}, apperr.Conflict(msgAlreadyWaived)
	case err != nil:
		return Penalties{}, apperr.Internal("Não foi possível perdoar a penalidade. Tente novamente em instantes.", err)
	}
	audit.Record(ctx, audit.Event{Action: "penalidade.perdoada", Result: audit.Success, Resource: &audit.Resource{Type: "projeto", ID: p.ID},
		Details: map[string]any{"penalidadeId": penaltyID}})
	return s.PenaltiesOfProject(ctx, reviewer, p.ID)
}
