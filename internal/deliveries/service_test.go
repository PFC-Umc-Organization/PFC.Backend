package deliveries

import (
	"context"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/storage"
)

type fakeStore struct {
	Store
	activities map[string]Activity
	saved      []Submission
	removed    []int
}

func (f *fakeStore) GetActivity(_ context.Context, id string) (Activity, bool, error) {
	a, ok := f.activities[id]
	return a, ok, nil
}
func (f *fakeStore) ListActivities(context.Context) ([]Activity, error) {
	out := []Activity{}
	for _, a := range f.activities {
		out = append(out, a)
	}
	return out, nil
}
func (f *fakeStore) RemoveField(_ context.Context, _ string, position int, _ string) error {
	f.removed = append(f.removed, position)
	return nil
}
func (f *fakeStore) PutSubmission(_ context.Context, s Submission, mustExist bool) error {
	for i, old := range f.saved {
		if old.ProjectID == s.ProjectID && old.ActivityID == s.ActivityID {
			f.saved[i] = s
			return nil
		}
	}
	if mustExist {
		return errNotFound
	}
	f.saved = append(f.saved, s)
	return nil
}

func (f *fakeStore) SubmissionsOfProject(_ context.Context, projectID string) ([]Submission, error) {
	out := []Submission{}
	for _, s := range f.saved {
		if s.ProjectID == projectID {
			out = append(out, s)
		}
	}
	return out, nil
}
func (f *fakeStore) GetSubmission(_ context.Context, projectID, activityID string) (Submission, bool, error) {
	for _, s := range f.saved {
		if s.ProjectID == projectID && s.ActivityID == activityID {
			return s, true, nil
		}
	}
	return Submission{}, false, nil
}
func (f *fakeStore) DeleteSubmission(_ context.Context, projectID, activityID string) (Submission, error) {
	for i, s := range f.saved {
		if s.ProjectID == projectID && s.ActivityID == activityID {
			f.saved = slices.Delete(f.saved, i, i+1)
			return s, nil
		}
	}
	return Submission{}, nil
}

type fakeProjects map[string]groups.Project

func (f fakeProjects) Project(_ context.Context, id string) (groups.Project, bool, error) {
	p, ok := f[id]
	return p, ok, nil
}

// ProjectsOfProgram returns every project; the advisor filter lives in groups.
func (f fakeProjects) ProjectsOfProgram(context.Context, actor.Actor, string) ([]groups.Project, error) {
	out := []groups.Project{}
	for _, p := range f {
		out = append(out, p)
	}
	return out, nil
}

func newTestService(t *testing.T, store *fakeStore) *Service {
	t.Helper()
	t.Cleanup(audit.SetOutput(io.Discard))
	svc := NewService(store, testProjects, nil)
	svc.now = func() time.Time { return time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC) }
	return svc
}

var testProjects = fakeProjects{
	"p1": {ID: "p1", Members: []string{"111"}, AdvisorID: "prof-sub"},
	"p2": {ID: "p2", Members: []string{"222"}},
}

func message(err error) string {
	if e, ok := apperr.As(err); ok {
		return e.Message
	}
	return ""
}

var tcc = Activity{ID: "a1", Fields: []Field{{ID: "f1", Label: "Arquivo", Type: FieldFile, Required: true}, {ID: "f2", Label: "Repo", Type: FieldLink}}}

func TestListActivitiesByDeadline(t *testing.T) {
	svc := newTestService(t, &fakeStore{activities: map[string]Activity{
		"b": {ID: "b", Deadline: "2026-11-01T10:00"}, "a": {ID: "a", Deadline: "2026-10-01T10:00"}, "c": {ID: "c", Deadline: "2026-12-01T10:00"},
	}})
	activities, _ := svc.ListActivities(context.Background())
	ids := []string{activities[0].ID, activities[1].ID, activities[2].ID}
	if !slices.Equal(ids, []string{"a", "b", "c"}) {
		t.Errorf("order = %v", ids)
	}
}

func TestRemoveField(t *testing.T) {
	store := &fakeStore{activities: map[string]Activity{"a1": tcc, "one": {ID: "one", Fields: tcc.Fields[:1]}}}
	svc := newTestService(t, store)
	ctx := context.Background()

	if err := svc.RemoveField(ctx, "a1", "f2"); err != nil || !slices.Equal(store.removed, []int{1}) {
		t.Fatalf("err = %v, removed = %v", err, store.removed)
	}
	if msg := message(svc.RemoveField(ctx, "one", "f1")); msg != msgLastField {
		t.Errorf("last field: %q", msg)
	}
	if msg := message(svc.RemoveField(ctx, "a1", "zz")); msg != msgFieldNotFound {
		t.Errorf("unknown field: %q", msg)
	}
	if msg := message(svc.RemoveField(ctx, "zz", "f1")); msg != msgActivityNotFound {
		t.Errorf("unknown activity: %q", msg)
	}
}

func TestSubmit(t *testing.T) {
	store := &fakeStore{activities: map[string]Activity{"a1": tcc}}
	svc := newTestService(t, store)
	ctx := context.Background()
	member := actor.Actor{Role: actor.RoleStudent, Email: "111@alunos.umc.br", Name: "Ana"}

	sub, err := svc.Submit(ctx, member, "p1", "a1", SubmissionInput{Answers: map[string]string{"f1": " tema.pdf "}})
	if err != nil || sub.SubmittedBy != "Ana" || sub.SubmittedAt != "2026-10-16T12:00:00Z" || sub.Answers["f1"] != "tema.pdf" || len(store.saved) != 1 {
		t.Fatalf("sub = %+v, err = %v", sub, err)
	}

	cases := map[string]struct {
		who      actor.Actor
		project  string
		activity string
		answers  map[string]string
		want     string
	}{
		"advisor":          {actor.Actor{Role: actor.RoleAdvisor, Email: "111@alunos.umc.br"}, "p1", "a1", nil, msgMembersOnly},
		"outsider":         {actor.Actor{Email: "222@alunos.umc.br"}, "p1", "a1", nil, msgMembersOnly},
		"missing project":  {member, "p9", "a1", nil, msgProjectNotFound},
		"missing activity": {member, "p1", "a9", nil, msgActivityNotFound},
		"required missing": {member, "p1", "a1", map[string]string{}, `O campo "Arquivo" é obrigatório`},
	}
	for name, c := range cases {
		_, err := svc.Submit(ctx, c.who, c.project, c.activity, SubmissionInput{Answers: c.answers})
		if message(err) != c.want {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if len(store.saved) != 1 {
		t.Errorf("refusals must not save: %d saved", len(store.saved))
	}
}

func TestDeadline(t *testing.T) {
	store := &fakeStore{activities: map[string]Activity{
		"open":   {ID: "open", Deadline: "2026-10-16T09:30", Fields: tcc.Fields},
		"closed": {ID: "closed", Deadline: "2026-10-16T08:59", Fields: tcc.Fields},
	}}
	svc := newTestService(t, store)
	ctx := context.Background()
	ana := actor.Actor{Role: actor.RoleStudent, Email: "111@alunos.umc.br", Name: "Ana"}
	bia := actor.Actor{Role: actor.RoleStudent, Email: "111@alunos.umc.br", Name: "Bia"}
	file := func(name string) SubmissionInput { return SubmissionInput{Answers: map[string]string{"f1": name}} }

	// 12:00 UTC is 09:00 in Brasília.
	if _, err := svc.Submit(ctx, ana, "p1", "open", file("v1.pdf")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Submit(ctx, ana, "p1", "closed", file("v1.pdf")); message(err) != msgDeadlinePassed {
		t.Errorf("first submission after the deadline: %v", err)
	}

	svc.now = func() time.Time { return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC) }
	sub, err := svc.Submit(ctx, bia, "p1", "open", file("v2.docx"))
	if err != nil || sub.SubmittedAt != "2026-10-16T12:00:00Z" || sub.SubmittedBy != "Ana" ||
		sub.UpdatedAt != "2026-10-20T12:00:00Z" || sub.UpdatedBy != "Bia" || sub.Answers["f1"] != "v2.docx" {
		t.Fatalf("editing after the deadline: %+v, %v", sub, err)
	}

	if err := svc.RemoveSubmission(ctx, actor.Actor{Role: actor.RoleAdmin}, "p1", "open"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Submit(ctx, ana, "p1", "open", file("v3.pdf")); message(err) != msgDeadlinePassed {
		t.Errorf("handed back after the deadline: %v", err)
	}
	if len(store.saved) != 0 {
		t.Errorf("saved = %+v", store.saved)
	}
}

func TestWritesAreStaffOnly(t *testing.T) {
	r := httpapi.NewRouter()
	Routes(r, newTestService(t, &fakeStore{}))
	for _, route := range [][2]string{
		{"POST", "/atividades"},
		{"PUT", "/atividades/a1"},
		{"DELETE", "/atividades/a1"},
		{"POST", "/atividades/a1/campos"},
		{"DELETE", "/atividades/a1/campos/f1"},
		{"GET", "/atividades/a1/entregas"},
		{"DELETE", "/projetos/p1/entregas/a1"},
	} {
		resp, _ := r.Lambda(context.Background(), events.APIGatewayProxyRequest{HTTPMethod: route[0], Path: route[1],
			RequestContext: events.APIGatewayProxyRequestContext{Authorizer: map[string]any{"claims": map[string]any{"custom:perfil": "ALUNO"}}}})
		if resp.StatusCode != 403 {
			t.Errorf("%s %s: student got %d", route[0], route[1], resp.StatusCode)
		}
	}
}

func TestSubmissionsOfProgram(t *testing.T) {
	store := &fakeStore{saved: []Submission{{ActivityID: "a1", ProjectID: "p1"}, {ActivityID: "a1", ProjectID: "p2"}, {ActivityID: "a2", ProjectID: "p1"}}}
	got, err := newTestService(t, store).SubmissionsOfProgram(context.Background(), actor.Actor{Role: actor.RoleAdmin}, "prog")
	if err != nil || len(got) != 3 {
		t.Fatalf("submissions = %+v, err = %v", got, err)
	}
	got, err = newTestService(t, &fakeStore{}).SubmissionsOfProgram(context.Background(), actor.Actor{Role: actor.RoleAdmin}, "prog")
	if err != nil || got == nil {
		t.Fatalf("empty program = %#v, err = %v; want [] so the JSON is not null", got, err)
	}
}

func TestRemoveSubmissionNeedsReviewer(t *testing.T) {
	svc := newTestService(t, &fakeStore{})
	ctx := context.Background()
	for name, c := range map[string]struct {
		who  actor.Actor
		kind apperr.Kind
	}{
		"admin":         {actor.Actor{Role: actor.RoleAdmin}, 0},
		"own advisor":   {actor.Actor{Role: actor.RoleAdvisor, Sub: "prof-sub"}, 0},
		"other advisor": {actor.Actor{Role: actor.RoleAdvisor, Sub: "outro"}, apperr.KindForbidden},
	} {
		err := svc.RemoveSubmission(ctx, c.who, "p1", "a1")
		var kind apperr.Kind
		if e, ok := apperr.As(err); ok {
			kind = e.Kind
		}
		if kind != c.kind {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if err := svc.RemoveSubmission(ctx, actor.Actor{Role: actor.RoleAdmin}, "missing", "a1"); message(err) != msgProjectNotFound {
		t.Errorf("missing project: %v", err)
	}
}

type fakeFiles struct {
	sizes   map[string]int64
	deleted []string
	folders []string
}

func (f *fakeFiles) PresignUpload(_ context.Context, key, contentType string, maxBytes int64, _ time.Duration) (storage.Upload, error) {
	return storage.Upload{URL: "https://bucket", Fields: map[string]string{"key": key, "Content-Type": contentType}}, nil
}
func (f *fakeFiles) PresignDownload(_ context.Context, key, name string, _ time.Duration) (string, error) {
	return "https://bucket/" + key + "?nome=" + name, nil
}
func (f *fakeFiles) Size(_ context.Context, key string) (int64, bool, error) {
	n, ok := f.sizes[key]
	return n, ok, nil
}
func (f *fakeFiles) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}
func (f *fakeFiles) DeletePrefix(_ context.Context, prefix string) error {
	f.folders = append(f.folders, prefix)
	return nil
}

func TestFiles(t *testing.T) {
	store := &fakeStore{activities: map[string]Activity{"a1": tcc}}
	files := &fakeFiles{sizes: map[string]int64{}}
	svc := newTestService(t, store)
	svc.files = files
	svc.newID = func() string { return "u1" }
	ctx := context.Background()
	member := actor.Actor{Role: actor.RoleStudent, Email: "111@alunos.umc.br"}

	ticket, err := svc.RequestUpload(ctx, member, "p1", "a1", UploadRequest{FieldID: "f1", Name: "Monografia Final.PDF", Size: 1 << 20})
	if err != nil || ticket.Key != "entregas/p1/a1/f1/u1/Monografia Final.PDF" || ticket.Fields["Content-Type"] != "application/pdf" {
		t.Fatalf("ticket = %+v, err = %v", ticket, err)
	}
	for name, in := range map[string]UploadRequest{
		"too big":    {FieldID: "f1", Name: "a.pdf", Size: storage.MaxBytes + 1},
		"empty":      {FieldID: "f1", Name: "a.pdf"},
		"executable": {FieldID: "f1", Name: "a.exe", Size: 10},
		"link field": {FieldID: "f2", Name: "a.pdf", Size: 10},
	} {
		if _, err := svc.RequestUpload(ctx, member, "p1", "a1", in); kind(err) != apperr.KindInvalid {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := svc.RequestUpload(ctx, actor.Actor{Role: actor.RoleStudent, Email: "222@alunos.umc.br"}, "p1", "a1",
		UploadRequest{FieldID: "f1", Name: "a.pdf", Size: 10}); kind(err) != apperr.KindForbidden {
		t.Errorf("outsider: %v", err)
	}

	if _, err := svc.Submit(ctx, member, "p1", "a1", SubmissionInput{Answers: map[string]string{"f1": ticket.Key}}); message(err) != `O arquivo do campo "Arquivo" não chegou. Envie de novo` {
		t.Errorf("file not uploaded: %v", err)
	}
	for _, forged := range []string{"monografia.pdf", "entregas/p2/a1/f1/x/a.pdf"} {
		if _, err := svc.Submit(ctx, member, "p1", "a1", SubmissionInput{Answers: map[string]string{"f1": forged}}); kind(err) != apperr.KindInvalid {
			t.Errorf("%q accepted: %v", forged, err)
		}
	}
	files.sizes[ticket.Key] = 1 << 20
	if _, err := svc.Submit(ctx, member, "p1", "a1", SubmissionInput{Answers: map[string]string{"f1": ticket.Key}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Submit(ctx, member, "p1", "a1", SubmissionInput{Answers: map[string]string{"f1": ticket.Key, "f2": "https://x.y"}}); err != nil || len(files.deleted) != 0 {
		t.Fatalf("resubmitting the same file: %v, deleted %v", err, files.deleted)
	}

	d, err := svc.DownloadFile(ctx, actor.Actor{Role: actor.RoleAdvisor, Sub: "prof-sub"}, "p1", "a1", "f1")
	if err != nil || d.Name != "Monografia Final.PDF" {
		t.Fatalf("download = %+v, %v", d, err)
	}
	if _, err := svc.DownloadFile(ctx, actor.Actor{Role: actor.RoleAdvisor, Sub: "outro"}, "p1", "a1", "f1"); kind(err) != apperr.KindForbidden {
		t.Errorf("other advisor downloading: %v", err)
	}
	if _, err := svc.DownloadFile(ctx, member, "p1", "a1", "f2"); kind(err) != apperr.KindNotFound {
		t.Errorf("link field download: %v", err)
	}

	second := "entregas/p1/a1/f1/u2/v2.docx"
	files.sizes[second] = 10
	if _, err := svc.Submit(ctx, member, "p1", "a1", SubmissionInput{Answers: map[string]string{"f1": second}}); err != nil ||
		!slices.Equal(files.deleted, []string{ticket.Key}) {
		t.Fatalf("replacing the file: %v, deleted %v", err, files.deleted)
	}
	if err := svc.RemoveSubmission(ctx, actor.Actor{Role: actor.RoleAdmin}, "p1", "a1"); err != nil ||
		!slices.Equal(files.deleted, []string{ticket.Key, second}) {
		t.Fatalf("removing the submission: %v, deleted %v", err, files.deleted)
	}
}

func TestWithoutStorage(t *testing.T) {
	store := &fakeStore{activities: map[string]Activity{"a1": tcc}}
	svc := newTestService(t, store)
	member := actor.Actor{Role: actor.RoleStudent, Email: "111@alunos.umc.br"}
	if _, err := svc.RequestUpload(context.Background(), member, "p1", "a1", UploadRequest{FieldID: "f1", Name: "a.pdf", Size: 1}); kind(err) != apperr.KindUnavailable {
		t.Errorf("upload without bucket: %v", err)
	}
	if _, err := svc.Submit(context.Background(), member, "p1", "a1", SubmissionInput{Answers: map[string]string{"f1": "monografia.pdf"}}); err != nil {
		t.Errorf("a bare name must still work without a bucket: %v", err)
	}
}

func kind(err error) apperr.Kind {
	if e, ok := apperr.As(err); ok {
		return e.Kind
	}
	return 0
}
