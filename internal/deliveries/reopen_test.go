package deliveries

import (
	"context"
	"testing"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
)

// ---------------------------------------------------------------------------
// Reabertura
// ---------------------------------------------------------------------------

// Perdão
func TestWaivedPenaltyReopens(t *testing.T) {
	member := actor.Actor{Role: actor.RoleStudent, Email: "111@alunos.umc.br", Name: "Ana"}
	input := SubmissionInput{Answers: map[string]string{"f1": "v1.pdf"}}
	waived := penaltyRecord{ID: "prazo-closed", Waiver: &Waiver{Reason: "Instabilidade no portal.", By: "Coord", At: "2026-10-15T10:00:00Z"}}
	closed := func(id string) Activity { return Activity{ID: id, Deadline: "2026-10-01T08:00", Fields: tcc.Fields} }

	cases := map[string]struct {
		activities map[string]Activity
		penalties  []penaltyRecord
		wantErr    string
	}{
		"waived": {
			activities: map[string]Activity{"closed": closed("closed")},
			penalties:  []penaltyRecord{waived},
		},
		"not waived": {
			activities: map[string]Activity{"closed": closed("closed")},
			wantErr:    msgDeadlinePassed,
		},
		"waived other activity": {
			activities: map[string]Activity{"closed": closed("closed"), "other": closed("other")},
			penalties:  []penaltyRecord{{ID: "prazo-other", Waiver: waived.Waiver}},
			wantErr:    msgDeadlinePassed,
		},
		"group failed": {
			activities: map[string]Activity{"closed": closed("closed"), "x1": closed("x1"), "x2": closed("x2"), "x3": closed("x3")},
			penalties:  []penaltyRecord{waived},
			wantErr:    msgGroupFailed,
		},
	}
	for name, c := range cases {
		store := &fakeStore{activities: c.activities, penalties: map[string][]penaltyRecord{"p1": c.penalties}}
		svc := newTestService(t, store)
		_, err := svc.Submit(context.Background(), member, "p1", "closed", input)
		if message(err) != c.wantErr {
			t.Errorf("%s: err = %v", name, err)
		}
		if c.wantErr == "" && len(store.saved) != 1 {
			t.Errorf("%s: saved %d", name, len(store.saved))
		}
	}
}

// Ticket
func TestWaivedPenaltyUpload(t *testing.T) {
	member := actor.Actor{Role: actor.RoleStudent, Email: "111@alunos.umc.br"}
	activities := map[string]Activity{"closed": {ID: "closed", Deadline: "2026-10-01T08:00", Fields: tcc.Fields}}
	request := UploadRequest{FieldID: "f1", Name: "a.pdf", Size: 10}

	for name, c := range map[string]struct {
		penalties []penaltyRecord
		wantErr   string
	}{
		"waived":     {[]penaltyRecord{{ID: "prazo-closed", Waiver: &Waiver{Reason: "ok"}}}, ""},
		"not waived": {nil, msgDeadlinePassed},
	} {
		svc := newTestService(t, &fakeStore{activities: activities, penalties: map[string][]penaltyRecord{"p1": c.penalties}})
		svc.files = &fakeFiles{sizes: map[string]int64{}}
		_, err := svc.RequestUpload(context.Background(), member, "p1", "closed", request)
		if message(err) != c.wantErr {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
