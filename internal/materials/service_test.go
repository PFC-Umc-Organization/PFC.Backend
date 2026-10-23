package materials

import (
	"context"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/storage"
)

type memStore struct{ items map[string]Material }

func (m *memStore) Create(_ context.Context, x Material) error { m.items[x.ID] = x; return nil }
func (m *memStore) Get(_ context.Context, id string) (Material, bool, error) {
	x, ok := m.items[id]
	return x, ok, nil
}
func (m *memStore) List(context.Context) ([]Material, error) {
	out := []Material{}
	for _, x := range m.items {
		out = append(out, x)
	}
	return out, nil
}
func (m *memStore) Delete(_ context.Context, id string) (Material, error) {
	x, ok := m.items[id]
	if !ok {
		return Material{}, errNotFound
	}
	delete(m.items, id)
	return x, nil
}

type academic map[string]string

func (a academic) CohortExists(_ context.Context, id string) (bool, error) {
	return id == "t1" || id == "t2", nil
}
func (a academic) CohortOf(_ context.Context, rgm string) (string, error) { return a[rgm], nil }

type files struct {
	sizes   map[string]int64
	deleted []string
}

func (f *files) PresignUpload(_ context.Context, key, ct string, _ int64, _ time.Duration) (storage.Upload, error) {
	return storage.Upload{URL: "https://bucket", Fields: map[string]string{"key": key, "Content-Type": ct}}, nil
}
func (f *files) PresignDownload(_ context.Context, key, name string, _ time.Duration) (string, error) {
	return "https://bucket/" + key, nil
}
func (f *files) Size(_ context.Context, key string) (int64, bool, error) {
	n, ok := f.sizes[key]
	return n, ok, nil
}
func (f *files) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}

var (
	admin   = actor.Actor{Role: actor.RoleAdmin, Sub: "adm", Name: "Coordenação"}
	advisor = actor.Actor{Role: actor.RoleAdvisor, Sub: "prof", Name: "Prof"}
	other   = actor.Actor{Role: actor.RoleAdvisor, Sub: "outro", Name: "Outro"}
)

func student(rgm string) actor.Actor {
	return actor.Actor{Role: actor.RoleStudent, Email: rgm + "@alunos.umc.br"}
}

func kind(err error) apperr.Kind {
	if e, ok := apperr.As(err); ok {
		return e.Kind
	}
	return 0
}

func newService(t *testing.T, f Files) (*Service, *memStore) {
	t.Helper()
	t.Cleanup(audit.SetOutput(io.Discard))
	store := &memStore{items: map[string]Material{}}
	svc := NewService(store, academic{"111": "t1", "222": "t2"}, f)
	n := 0
	svc.newID = func() string { n++; return string(rune('a' + n - 1)) }
	tick := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { tick = tick.Add(time.Minute); return tick }
	return svc, store
}

func ptr(s string) *string { return &s }

func TestVisibility(t *testing.T) {
	svc, _ := newService(t, nil)
	ctx := context.Background()
	for _, in := range []MaterialInput{
		{Title: "Geral", Kind: KindLink, URL: "https://x"},
		{Title: "Da t1", Kind: KindVideo, URL: "https://y", CohortID: ptr("t1")},
		{Title: "Da t2", Kind: KindLink, URL: "https://z", CohortID: ptr(" t2 ")},
		{Title: "Geral também", Kind: KindLink, URL: "https://w", CohortID: ptr(" ")},
	} {
		if _, err := svc.Create(ctx, advisor, in); err != nil {
			t.Fatal(err)
		}
	}
	titles := func(who actor.Actor) []string {
		list, err := svc.List(ctx, who)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, m := range list {
			out = append(out, m.Title)
		}
		return out
	}
	if got := titles(student("111")); !slices.Equal(got, []string{"Geral também", "Da t1", "Geral"}) {
		t.Errorf("student of t1 sees %v", got)
	}
	if got := titles(student("999")); !slices.Equal(got, []string{"Geral também", "Geral"}) {
		t.Errorf("student without cohort sees %v", got)
	}
	if got := titles(admin); len(got) != 4 {
		t.Errorf("admin sees %v", got)
	}
}

func TestCreateValidation(t *testing.T) {
	svc, _ := newService(t, nil)
	ctx := context.Background()
	for name, c := range map[string]struct {
		in   MaterialInput
		kind apperr.Kind
	}{
		"no title":         {MaterialInput{Kind: KindLink, URL: "https://x"}, apperr.KindInvalid},
		"bad kind":         {MaterialInput{Title: "x", Kind: "PODCAST", URL: "https://x"}, apperr.KindInvalid},
		"neither":          {MaterialInput{Title: "x", Kind: KindLink}, apperr.KindInvalid},
		"both":             {MaterialInput{Title: "x", Kind: KindDocument, URL: "https://x", File: "materiais/a/b.pdf"}, apperr.KindInvalid},
		"javascript link":  {MaterialInput{Title: "x", Kind: KindLink, URL: "javascript:alert(1)"}, apperr.KindInvalid},
		"forged file":      {MaterialInput{Title: "x", Kind: KindDocument, File: "entregas/p/a/f/u/x.pdf"}, apperr.KindInvalid},
		"unknown cohort":   {MaterialInput{Title: "x", Kind: KindLink, URL: "https://x", CohortID: ptr("t9")}, apperr.KindNotFound},
		"file, no storage": {MaterialInput{Title: "x", Kind: KindDocument, File: "materiais/a/b.pdf"}, apperr.KindUnavailable},
	} {
		if _, err := svc.Create(ctx, admin, c.in); kind(err) != c.kind {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestFileMaterial(t *testing.T) {
	f := &files{sizes: map[string]int64{}}
	svc, _ := newService(t, f)
	ctx := context.Background()

	if _, err := svc.RequestUpload(ctx, UploadRequest{Name: "x.exe", Size: 1}); kind(err) != apperr.KindInvalid {
		t.Errorf("exe: %v", err)
	}
	ticket, err := svc.RequestUpload(ctx, UploadRequest{Name: "Modelo PFC.docx", Size: 10})
	if err != nil || ticket.Key != "materiais/a/Modelo PFC.docx" {
		t.Fatalf("ticket = %+v, %v", ticket, err)
	}
	in := MaterialInput{Title: "Modelo", Kind: KindTemplate, File: ticket.Key, CohortID: ptr("t1")}
	if _, err := svc.Create(ctx, advisor, in); kind(err) != apperr.KindInvalid {
		t.Errorf("file not uploaded: %v", err)
	}
	f.sizes[ticket.Key] = 10
	m, err := svc.Create(ctx, advisor, in)
	if err != nil || m.File != "Modelo PFC.docx" || m.PublishedBy != "Prof" {
		t.Fatalf("material = %+v, %v", m, err)
	}

	if d, err := svc.Download(ctx, student("111"), m.ID); err != nil || d.Name != "Modelo PFC.docx" {
		t.Errorf("student of the cohort: %+v, %v", d, err)
	}
	if _, err := svc.Download(ctx, student("222"), m.ID); kind(err) != apperr.KindNotFound {
		t.Errorf("student of another cohort: %v", err)
	}

	if err := svc.Delete(ctx, other, m.ID); kind(err) != apperr.KindForbidden {
		t.Errorf("another advisor removing: %v", err)
	}
	if err := svc.Delete(ctx, advisor, m.ID); err != nil || !slices.Equal(f.deleted, []string{ticket.Key}) {
		t.Fatalf("author removing: %v, deleted %v", err, f.deleted)
	}
	if err := svc.Delete(ctx, admin, m.ID); kind(err) != apperr.KindNotFound {
		t.Errorf("removing twice: %v", err)
	}
}

func TestAdminRemovesAny(t *testing.T) {
	svc, _ := newService(t, nil)
	ctx := context.Background()
	m, _ := svc.Create(ctx, advisor, MaterialInput{Title: "Guia", Kind: KindLink, URL: "https://x"})
	if err := svc.Delete(ctx, admin, m.ID); err != nil {
		t.Fatal(err)
	}
}
