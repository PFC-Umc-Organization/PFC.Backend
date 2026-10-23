package deliveries

import (
	"context"
	"testing"
	"time"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
)

// The test clock is 2026-10-16 12:00 UTC.
func missed(id string) Activity {
	return Activity{ID: id, Title: "Atividade " + id, Deadline: "2026-10-0" + id[1:] + "T23:59", Fields: tcc.Fields}
}

func TestPenaltiesOf(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	activities := []Activity{missed("a1"), missed("a2"), {ID: "open", Deadline: "2026-11-01T23:59"}, {ID: "sem-prazo"}}
	got := penaltiesOf("p1", activities, []Submission{{ActivityID: "a2"}}, nil, now)
	if got.Active != 1 || got.Failed || len(got.Items) != 1 || got.Items[0].ID != "prazo-a1" || got.Items[0].AppliedAt != "2026-10-02T02:59:00Z" {
		t.Fatalf("only the missed deadline counts, at Brasília time: %+v", got)
	}

	records := []penaltyRecord{
		{ID: "prazo-a1", Waiver: &Waiver{Reason: "Atestado"}},
		{ID: "manual-1", Reason: "Entrega não validada", AppliedAt: "2026-10-03T12:00:00Z"},
		{ID: "manual-2", Reason: "Plágio", AppliedAt: "2026-10-04T12:00:00Z"},
		{ID: "prazo-apagada", Waiver: &Waiver{Reason: "Atividade removida depois"}},
	}
	got = penaltiesOf("p1", activities, nil, records, now)
	if got.Active != 3 || !got.Failed || len(got.Items) != 4 || got.Items[0].Waiver == nil {
		t.Fatalf("a waiver keeps the penalty listed but inactive; an orphan waiver is ignored: %+v", got)
	}
}

func failedStore() *fakeStore {
	return &fakeStore{
		activities: map[string]Activity{"a1": missed("a1"), "a2": missed("a2"), "a3": missed("a3"), "open": {ID: "open", Deadline: "2099-01-01T23:59", Fields: []Field{{ID: "f2", Label: "Repo", Type: FieldLink, Required: true}}}},
	}
}

func TestFailedGroupCannotSubmitUntilAWaiver(t *testing.T) {
	store := failedStore()
	svc := newTestService(t, store)
	svc.files = &fakeFiles{sizes: map[string]int64{}}
	ctx := context.Background()
	member := actor.Actor{Role: actor.RoleStudent, Email: "111@alunos.umc.br"}
	answers := SubmissionInput{Answers: map[string]string{"f2": "https://github.com/umc/athena"}}

	_, err := svc.Submit(ctx, member, "p1", "open", answers)
	if message(err) != msgGroupFailed {
		t.Fatalf("three missed deadlines fail the group: err = %v", err)
	}
	if _, err := svc.RequestUpload(ctx, member, "p1", "open", UploadRequest{FieldID: "f2", Name: "a.pdf", Size: 10}); message(err) != msgGroupFailed {
		t.Fatalf("upload: err = %v", err)
	}

	got, err := svc.WaivePenalty(ctx, actor.Actor{Role: actor.RoleAdvisor, Sub: "prof-sub", Name: "Prof"}, "p1", "prazo-a2", WaiverInput{Reason: " Atestado médico "})
	if err != nil || got.Active != 2 || got.Failed {
		t.Fatalf("waive = %+v, err = %v", got, err)
	}
	if w := got.Items[1].Waiver; w == nil || w.Reason != "Atestado médico" || w.By != "Prof" {
		t.Fatalf("waiver = %+v", w)
	}
	if _, err := svc.Submit(ctx, member, "p1", "open", answers); err != nil {
		t.Fatalf("back under the limit the group submits again: %v", err)
	}
	if _, err := svc.WaivePenalty(ctx, actor.Actor{Role: actor.RoleAdmin}, "p1", "prazo-a2", WaiverInput{Reason: "De novo"}); kind(err) != apperr.KindConflict {
		t.Fatalf("a waived penalty is not waived twice: %v", err)
	}
	if _, err := svc.WaivePenalty(ctx, actor.Actor{Role: actor.RoleAdmin}, "p1", "prazo-open", WaiverInput{Reason: "x"}); kind(err) != apperr.KindNotFound {
		t.Fatalf("an open activity has no penalty: %v", err)
	}
}

func TestApplyPenalty(t *testing.T) {
	store := &fakeStore{activities: map[string]Activity{"a1": tcc}}
	svc := newTestService(t, store)
	ctx := context.Background()
	admin := actor.Actor{Role: actor.RoleAdmin, Name: "Coordenação"}

	got, err := svc.ApplyPenalty(ctx, admin, "p1", PenaltyInput{Reason: "Entrega não validada", ActivityID: "a1"})
	if err != nil || got.Active != 1 || got.Items[0].Source != PenaltyManual || got.Items[0].AppliedBy != "Coordenação" || got.Items[0].ActivityTitle != tcc.Title {
		t.Fatalf("apply = %+v, err = %v", got, err)
	}
	for name, c := range map[string]struct {
		who  actor.Actor
		in   PenaltyInput
		kind apperr.Kind
	}{
		"no reason":        {admin, PenaltyInput{Reason: "  "}, apperr.KindInvalid},
		"unknown activity": {admin, PenaltyInput{Reason: "x", ActivityID: "nada"}, apperr.KindNotFound},
		"other advisor":    {actor.Actor{Role: actor.RoleAdvisor, Sub: "outro"}, PenaltyInput{Reason: "x"}, apperr.KindForbidden},
		"member":           {actor.Actor{Role: actor.RoleStudent, Email: "111@alunos.umc.br"}, PenaltyInput{Reason: "x"}, apperr.KindForbidden},
	} {
		if _, err := svc.ApplyPenalty(ctx, c.who, "p1", c.in); kind(err) != c.kind {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if len(store.penalties["p1"]) != 1 {
		t.Fatalf("refused penalties must not be stored: %+v", store.penalties)
	}
}

func TestPenaltiesVisibility(t *testing.T) {
	svc := newTestService(t, failedStore())
	ctx := context.Background()
	if got, err := svc.PenaltiesOfProject(ctx, actor.Actor{Role: actor.RoleStudent, Email: "111@alunos.umc.br"}, "p1"); err != nil || !got.Failed {
		t.Fatalf("member: %+v, %v", got, err)
	}
	if _, err := svc.PenaltiesOfProject(ctx, actor.Actor{Role: actor.RoleStudent, Email: "222@alunos.umc.br"}, "p1"); kind(err) != apperr.KindForbidden {
		t.Fatalf("another group's student: %v", err)
	}
	got, err := svc.PenaltiesOfProgram(ctx, actor.Actor{Role: actor.RoleAdmin}, "prog")
	if err != nil || len(got) != 2 {
		t.Fatalf("program = %+v, err = %v", got, err)
	}
}
