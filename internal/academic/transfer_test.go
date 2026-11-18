package academic

import (
	"bytes"
	"context"
	"testing"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

// ---------------------------------------------------------------------------
// Dublês
// ---------------------------------------------------------------------------

// Simulação
type memberProjects map[string]string

// Verificação
func (memberProjects) ProgramHasProjects(context.Context, string) (bool, error) { return false, nil }

// Consulta
func (m memberProjects) ProgramOfMember(_ context.Context, rgm string) (string, bool, error) {
	p, ok := m[rgm]
	return p, ok, nil
}

// ---------------------------------------------------------------------------
// Transferência
// ---------------------------------------------------------------------------

// Bloqueio
func TestEnrollTransferWithProject(t *testing.T) {
	cohorts := map[string]Cohort{
		"morning":   {ID: "morning", CourseID: "es", Shift: "MANHA", Term: "2026.2"},
		"night":     {ID: "night", CourseID: "es", Shift: "NOITE", Term: "2026.2"},
		"old-term":  {ID: "old-term", CourseID: "es", Shift: "NOITE", Term: "2026.1"},
		"other-cor": {ID: "other-cor", CourseID: "si", Shift: "NOITE", Term: "2026.2"},
	}
	cases := map[string]struct {
		rgm    string
		target string
		failed bool
	}{
		"same course and term": {"111", "night", false},
		"same cohort":          {"111", "morning", false},
		"other term":           {"111", "old-term", true},
		"other course":         {"111", "other-cor", true},
		"no project":           {"222", "other-cor", false},
	}
	for name, c := range cases {
		store := &fakeStore{
			cohorts:  cohorts,
			programs: map[string]Program{"prog": {ID: "prog", CohortID: "morning"}},
			enrolled: map[string]string{},
		}
		var buf bytes.Buffer
		restore := audit.SetOutput(&buf)
		svc := NewService(store, memberProjects{"111": "prog"})
		result, err := svc.Enroll(context.Background(), EnrollmentRequest{RGMs: []string{c.rgm}, CohortID: c.target})
		restore()
		if c.failed {
			wantErr(t, err, apperr.KindConflict, msgEnrollInProject)
			if len(store.written) != 0 {
				t.Errorf("%s: wrote %v", name, store.written)
			}
			continue
		}
		if err != nil || result.Processed != 1 || len(store.written) != 1 {
			t.Errorf("%s: result = %+v, err = %v", name, result, err)
		}
	}
}
