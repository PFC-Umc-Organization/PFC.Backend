package references

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

type fakeWorks struct {
	works map[string]Work
	down  bool
}

func (f fakeWorks) Search(context.Context, string, int) (int, []Work, error) {
	if f.down {
		return 0, nil, errUnavailable
	}
	return 1, []Work{f.works[knownDOI]}, nil
}

func (f fakeWorks) WorkByDOI(_ context.Context, doi string) (Work, error) {
	if f.down {
		return Work{}, errUnavailable
	}
	w, ok := f.works[doi]
	if !ok {
		return Work{}, errWorkNotFound
	}
	return w, nil
}

type memoryStore struct {
	refs map[string]Reference
}

func (m *memoryStore) PutReference(_ context.Context, r Reference, _ string) error {
	if _, ok := m.refs[r.ID]; ok {
		return errDuplicate
	}
	m.refs[r.ID] = r
	return nil
}
func (m *memoryStore) ReferencesOf(context.Context, string) ([]Reference, error) {
	out := []Reference{}
	for _, r := range m.refs {
		out = append(out, r)
	}
	return out, nil
}
func (m *memoryStore) DeleteReference(_ context.Context, _, id string) error {
	if _, ok := m.refs[id]; !ok {
		return errNotFound
	}
	delete(m.refs, id)
	return nil
}

type fakeProjects map[string]groups.Project

func (f fakeProjects) Project(_ context.Context, id string) (groups.Project, bool, error) {
	p, ok := f[id]
	return p, ok, nil
}

const knownDOI = "10.1145/3290605.3300233"

var chi = Work{
	DOI: knownDOI, Type: WorkConferencePaper, Title: "Guidelines for Human-AI Interaction",
	Venue:     "Proceedings of the 2019 CHI Conference on Human Factors in Computing Systems",
	Publisher: "ACM", Place: "New York, NY, USA", Pages: "1-13", Year: 2019, Citations: 1234,
	Authors: []Author{{Surname: "Amershi", GivenName: "Saleema"}, {Surname: "Weld", GivenName: "Dan"}, {Organization: "Microsoft Research"}},
}

func newTestService(t *testing.T, works fakeWorks) (*Service, *memoryStore) {
	t.Helper()
	t.Cleanup(audit.SetOutput(io.Discard))
	store := &memoryStore{refs: map[string]Reference{}}
	svc := NewService(store, works, fakeProjects{"p1": {ID: "p1", Members: []string{"11111111"}}})
	svc.now = func() time.Time { return accessDate }
	return svc, store
}

func kind(err error) apperr.Kind {
	if e, ok := apperr.As(err); ok {
		return e.Kind
	}
	return 0
}

func TestSearchValidation(t *testing.T) {
	svc, _ := newTestService(t, fakeWorks{})
	for name, q := range map[string][2]string{
		"no term":       {"", ""},
		"short term":    {"ab", ""},
		"only spaces":   {"     ", ""},
		"long term":     {strings.Repeat("a", maxSearchLen+1), ""},
		"page not int":  {"gestão", "x"},
		"page zero":     {"gestão", "0"},
		"page past max": {"gestão", "51"},
	} {
		if _, err := svc.Search(context.Background(), q[0], q[1]); kind(err) != apperr.KindInvalid {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestServiceSearch(t *testing.T) {
	svc, _ := newTestService(t, fakeWorks{works: map[string]Work{knownDOI: chi}})
	result, err := svc.Search(context.Background(), " gestão ", "2")
	if err != nil || result.Page != 2 || len(result.Works) != 1 || result.Works[0].Authors[2] != "MICROSOFT RESEARCH" {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	down, _ := newTestService(t, fakeWorks{down: true})
	if _, err := down.Search(context.Background(), "gestão", ""); kind(err) != apperr.KindUnavailable {
		t.Errorf("service down: %v", err)
	}
}

func TestLookupDOI(t *testing.T) {
	svc, _ := newTestService(t, fakeWorks{works: map[string]Work{knownDOI: chi}})
	ctx := context.Background()

	got, err := svc.LookupDOI(ctx, "https://doi.org/"+knownDOI)
	if err != nil {
		t.Fatal(err)
	}
	want := "AMERSHI, Saleema; WELD, Dan; MICROSOFT RESEARCH. Guidelines for Human-AI Interaction. In: " +
		"Proceedings of the 2019 CHI Conference on Human Factors in Computing Systems. New York, NY, USA: ACM, 2019. " +
		"p. 1-13. DOI: 10.1145/3290605.3300233. Disponível em: https://doi.org/10.1145/3290605.3300233. Acesso em: 26 set. 2026."
	if got.ABNT != want || !strings.Contains(got.ABNTHTML, "<strong>Proceedings") || got.Citations != 1234 {
		t.Errorf("got %+v", got)
	}
	if _, err := svc.LookupDOI(ctx, "10.1145/nao-existe"); kind(err) != apperr.KindNotFound {
		t.Errorf("unknown DOI: %v", err)
	}
	if _, err := svc.LookupDOI(ctx, "isso não é doi"); kind(err) != apperr.KindInvalid {
		t.Errorf("invalid DOI: %v", err)
	}
}

func TestAddAndRemove(t *testing.T) {
	svc, store := newTestService(t, fakeWorks{works: map[string]Work{knownDOI: chi}})
	ctx := context.Background()
	member := actor.Actor{Email: "11111111@alunos.umc.br", Name: "Ana", Sub: "s1"}

	ref, err := svc.Add(ctx, member, "p1", ReferenceInput{DOI: "doi:" + knownDOI})
	if err != nil || ref.ID != referenceID(knownDOI) || ref.AddedBy != "Ana" || ref.AddedAt != "2026-09-26T15:00:00Z" || len(store.refs) != 1 {
		t.Fatalf("ref = %+v, err = %v", ref, err)
	}
	if _, err := svc.Add(ctx, member, "p1", ReferenceInput{DOI: knownDOI}); kind(err) != apperr.KindConflict {
		t.Errorf("duplicate: %v", err)
	}
	if err := svc.Remove(ctx, member, "p1", ref.ID); err != nil || len(store.refs) != 0 {
		t.Fatalf("remove: %v", err)
	}
	if err := svc.Remove(ctx, member, "p1", ref.ID); kind(err) != apperr.KindNotFound {
		t.Errorf("remove twice: %v", err)
	}
}

func TestPermissions(t *testing.T) {
	cases := []struct {
		name       string
		who        actor.Actor
		view, edit bool
	}{
		{"member without custom:perfil", actor.Actor{Email: "11111111@alunos.umc.br"}, true, true},
		{"member with ALUNO", actor.Actor{Role: actor.RoleStudent, Email: "11111111@alunos.umc.br"}, true, true},
		{"student of another group", actor.Actor{Email: "33333333@alunos.umc.br"}, false, false},
		{"student without e-mail", actor.Actor{}, false, false},
		{"advisor", actor.Actor{Role: actor.RoleAdvisor, Email: "alessandro.horas@umc.br"}, true, false},
		{"admin whose user matches an RGM", actor.Actor{Role: actor.RoleAdmin, Email: "11111111@umc.br"}, true, false},
	}
	for _, c := range cases {
		svc, _ := newTestService(t, fakeWorks{works: map[string]Work{knownDOI: chi}})
		_, err := svc.ReferencesOf(context.Background(), c.who, "p1")
		if (err == nil) != c.view {
			t.Errorf("%s: view err = %v", c.name, err)
		}
		_, err = svc.Add(context.Background(), c.who, "p1", ReferenceInput{DOI: knownDOI})
		if (err == nil) != c.edit || (!c.edit && kind(err) != apperr.KindForbidden) {
			t.Errorf("%s: edit err = %v", c.name, err)
		}
	}
}

func TestReferencesSortedAlphabetically(t *testing.T) {
	svc, store := newTestService(t, fakeWorks{})
	store.refs = map[string]Reference{"b": {ID: "b", ABNT: "souza"}, "a": {ID: "a", ABNT: "ALVES"}, "c": {ID: "c", ABNT: "Lima"}}
	refs, err := svc.ReferencesOf(context.Background(), actor.Actor{Role: actor.RoleAdmin}, "p1")
	if err != nil || refs[0].ID != "a" || refs[1].ID != "c" || refs[2].ID != "b" {
		t.Fatalf("refs = %+v, err = %v", refs, err)
	}
}
