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
func (f *fakeStore) PutSubmission(_ context.Context, s Submission) error {
	f.saved = append(f.saved, s)
	return nil
}

type fakeProjects map[string]groups.Project

func (f fakeProjects) Project(_ context.Context, id string) (groups.Project, bool, error) {
	p, ok := f[id]
	return p, ok, nil
}

func newTestService(t *testing.T, store *fakeStore) *Service {
	t.Helper()
	t.Cleanup(audit.SetOutput(io.Discard))
	svc := NewService(store, fakeProjects{"p1": {ID: "p1", Members: []string{"111"}}})
	svc.now = func() time.Time { return time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC) }
	return svc
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
		"required missing": {member, "p1", "a1", map[string]string{}, `o campo "Arquivo" é obrigatório`},
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
