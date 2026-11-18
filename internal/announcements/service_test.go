package announcements

import (
	"context"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

// ---------------------------------------------------------------------------
// Dublês
// ---------------------------------------------------------------------------

// Simulação
type fakeStore struct {
	items map[string]Announcement
}

// Criação
func (f *fakeStore) Create(_ context.Context, a Announcement) error {
	if _, ok := f.items[a.ID]; ok {
		return errDuplicate
	}
	f.items[a.ID] = a
	return nil
}

// Consulta
func (f *fakeStore) Get(_ context.Context, id string) (Announcement, bool, error) {
	a, ok := f.items[id]
	return a, ok, nil
}

// Listagem
func (f *fakeStore) List(context.Context) ([]Announcement, error) {
	out := []Announcement{}
	for _, a := range f.items {
		out = append(out, a)
	}
	return out, nil
}

// Exclusão
func (f *fakeStore) Delete(_ context.Context, id string) error {
	if _, ok := f.items[id]; !ok {
		return errNotFound
	}
	delete(f.items, id)
	return nil
}

// Simulação
type fakeAcademic struct{ cohorts map[string]string }

// Verificação
func (f fakeAcademic) CohortExists(_ context.Context, id string) (bool, error) {
	return id == "t1" || id == "t2" || id == "t3", nil
}

// Consulta
func (f fakeAcademic) CohortOf(_ context.Context, rgm string) (string, error) {
	return f.cohorts[rgm], nil
}

// Consulta
func (f fakeAcademic) CohortsOf(_ context.Context, rgms []string) (map[string]string, error) {
	out := map[string]string{}
	for _, r := range rgms {
		out[r] = f.cohorts[r]
	}
	return out, nil
}

// Simulação
type fakeAdvising map[string][]string

// Consulta
func (f fakeAdvising) MembersAdvisedBy(_ context.Context, sub string) ([]string, error) {
	return f[sub], nil
}

// ---------------------------------------------------------------------------
// Auxiliares
// ---------------------------------------------------------------------------

var (
	admin    = actor.Actor{Role: actor.RoleAdmin, Sub: "admin", Name: "Coordenação"}
	advisor  = actor.Actor{Role: actor.RoleAdvisor, Sub: "adv1", Name: "Prof. Ana"}
	advisor2 = actor.Actor{Role: actor.RoleAdvisor, Sub: "adv2", Name: "Prof. Bia"}
	student  = actor.Actor{Role: actor.RoleStudent, Email: "111@alunos.umc.br"}
)

// Construção
func newTestService(t *testing.T) (*Service, *fakeStore) {
	t.Helper()
	t.Cleanup(audit.SetOutput(io.Discard))
	store := &fakeStore{items: map[string]Announcement{}}
	svc := NewService(store,
		fakeAcademic{cohorts: map[string]string{"111": "t1", "222": "t2", "333": "t2"}},
		fakeAdvising{"adv1": {"111", "222"}, "adv2": {"333"}})
	n := 0
	svc.newID = func() string { n++; return string(rune('a' + n - 1)) }
	svc.now = func() time.Time { return time.Date(2026, 11, 1, 10, 0, n, 0, time.UTC) }
	return svc, store
}

// Mensagem
func message(err error) string {
	if e, ok := apperr.As(err); ok {
		return e.Message
	}
	return ""
}

// Identificadores
func ids(list []Announcement) []string {
	out := []string{}
	for _, a := range list {
		out = append(out, a.ID)
	}
	slices.Sort(out)
	return out
}

// ---------------------------------------------------------------------------
// Publicação
// ---------------------------------------------------------------------------

// Papéis
func TestCreate(t *testing.T) {
	cases := map[string]struct {
		who     actor.Actor
		in      AnnouncementInput
		wantErr string
	}{
		"admin general":        {admin, AnnouncementInput{Title: "Geral", Content: "Texto"}, ""},
		"admin cohort":         {admin, AnnouncementInput{Title: "Turma", Content: "Texto", CohortID: "t3"}, ""},
		"admin unknown cohort": {admin, AnnouncementInput{Title: "Turma", Content: "Texto", CohortID: "zz"}, msgCohortNotFound},
		"advisor own cohort":   {advisor, AnnouncementInput{Title: "Turma", Content: "Texto", CohortID: "t1"}, ""},
		"advisor other cohort": {advisor, AnnouncementInput{Title: "Turma", Content: "Texto", CohortID: "t3"}, msgAdvisorCohort},
		"advisor general":      {advisor, AnnouncementInput{Title: "Geral", Content: "Texto"}, msgAdvisorCohort},
		"missing title":        {admin, AnnouncementInput{Content: "Texto"}, msgTitleRequired},
		"missing content":      {admin, AnnouncementInput{Title: "Título"}, msgContentRequired},
	}
	for name, c := range cases {
		svc, store := newTestService(t)
		a, err := svc.Create(context.Background(), c.who, c.in)
		if message(err) != c.wantErr {
			t.Errorf("%s: err = %v", name, err)
			continue
		}
		if c.wantErr == "" && (len(store.items) != 1 || a.AuthorID != c.who.Sub || a.AuthorRole != string(c.who.Role) || a.AuthorName != c.who.Name) {
			t.Errorf("%s: saved %+v", name, a)
		}
		if c.wantErr != "" && len(store.items) != 0 {
			t.Errorf("%s: refusal must not save", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Listagem
// ---------------------------------------------------------------------------

// Visibilidade
func TestList(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	for _, in := range []AnnouncementInput{
		{Title: "Geral", Content: "x"},
		{Title: "T1", Content: "x", CohortID: "t1"},
		{Title: "T2", Content: "x", CohortID: "t2"},
		{Title: "T3", Content: "x", CohortID: "t3"},
	} {
		if _, err := svc.Create(ctx, admin, in); err != nil {
			t.Fatal(err)
		}
	}
	cases := map[string]struct {
		who    actor.Actor
		cohort string
		want   []string
	}{
		"admin all":       {admin, "", []string{"a", "b", "c", "d"}},
		"admin filtered":  {admin, "t3", []string{"d"}},
		"advisor":         {advisor, "", []string{"a", "b", "c"}},
		"advisor other":   {advisor2, "", []string{"a", "c"}},
		"advisor filter":  {advisor, "t1", []string{"b"}},
		"student":         {student, "", []string{"a", "b"}},
		"student foreign": {student, "t2", []string{}},
		"no cohort":       {actor.Actor{Role: actor.RoleStudent, Email: "999@alunos.umc.br"}, "", []string{"a"}},
	}
	for name, c := range cases {
		list, err := svc.List(ctx, c.who, c.cohort)
		if err != nil || !slices.Equal(ids(list), c.want) {
			t.Errorf("%s: got %v, err = %v", name, ids(list), err)
		}
	}
}

// ---------------------------------------------------------------------------
// Exclusão
// ---------------------------------------------------------------------------

// Papéis
func TestDelete(t *testing.T) {
	svc, store := newTestService(t)
	ctx := context.Background()
	own, _ := svc.Create(ctx, advisor, AnnouncementInput{Title: "Dela", Content: "x", CohortID: "t1"})
	other, _ := svc.Create(ctx, admin, AnnouncementInput{Title: "Geral", Content: "x"})

	if msg := message(svc.Delete(ctx, advisor2, own.ID)); msg != msgRemoveOwn {
		t.Errorf("other advisor: %q", msg)
	}
	if msg := message(svc.Delete(ctx, advisor, other.ID)); msg != msgRemoveOwn {
		t.Errorf("advisor on admin notice: %q", msg)
	}
	if msg := message(svc.Delete(ctx, admin, "zz")); msg != msgNotFound {
		t.Errorf("unknown: %q", msg)
	}
	if err := svc.Delete(ctx, advisor, own.ID); err != nil {
		t.Errorf("author: %v", err)
	}
	if err := svc.Delete(ctx, admin, other.ID); err != nil || len(store.items) != 0 {
		t.Errorf("admin: %v, left %d", err, len(store.items))
	}
}
