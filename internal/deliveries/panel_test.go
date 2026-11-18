package deliveries

import (
	"context"
	"testing"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
)

// ---------------------------------------------------------------------------
// Painel
// ---------------------------------------------------------------------------

// Agregação
func TestPanel(t *testing.T) {
	store := &fakeStore{
		activities: map[string]Activity{"a1": {ID: "a1", Deadline: "2026-10-01T08:00", Fields: tcc.Fields}},
		saved:      []Submission{{ActivityID: "a1", ProjectID: "p1", SubmittedAt: "2026-09-30T10:00:00Z"}},
	}
	svc := newTestService(t, store)
	panel, err := svc.Panel(context.Background(), actor.Actor{Role: actor.RoleAdmin}, "prog")
	if err != nil || len(panel.Projects) != 2 || len(panel.Activities) != 1 || len(panel.Submissions) != 1 || len(panel.Penalties) != 2 {
		t.Fatalf("panel = %+v, err = %v", panel, err)
	}
	active := map[string]int{}
	for _, p := range panel.Penalties {
		active[p.ProjectID] = p.Active
	}
	if active["p1"] != 0 || active["p2"] != 1 {
		t.Errorf("active penalties = %v", active)
	}
}

// Vazio
func TestPanelEmpty(t *testing.T) {
	svc := newTestService(t, &fakeStore{})
	svc.projects = fakeProjects{}
	panel, err := svc.Panel(context.Background(), actor.Actor{Role: actor.RoleAdmin}, "prog")
	if err != nil || panel.Projects == nil || panel.Activities == nil || panel.Submissions == nil || panel.Penalties == nil ||
		len(panel.Projects)+len(panel.Activities)+len(panel.Submissions)+len(panel.Penalties) != 0 {
		t.Fatalf("panel = %+v, err = %v", panel, err)
	}
}
