package academic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

type fakeStore struct {
	Store
	cohorts  map[string]Cohort
	programs map[string]Program
	err      error
	written  [][]string
}

func (f *fakeStore) GetCohort(_ context.Context, id string) (Cohort, bool, error) {
	c, ok := f.cohorts[id]
	return c, ok, nil
}
func (f *fakeStore) CreateCohort(context.Context, Cohort) error         { return f.err }
func (f *fakeStore) UpdateCohort(context.Context, Cohort, Cohort) error { return f.err }
func (f *fakeStore) DeleteCohort(context.Context, Cohort) error         { return f.err }
func (f *fakeStore) CohortExists(_ context.Context, id string) (bool, error) {
	_, ok := f.cohorts[id]
	return ok, nil
}
func (f *fakeStore) GetProgram(_ context.Context, id string) (Program, bool, error) {
	p, ok := f.programs[id]
	return p, ok, nil
}
func (f *fakeStore) DeleteProgram(context.Context, Program) error { return f.err }
func (f *fakeStore) PutEnrollments(_ context.Context, rgms []string, _ string) error {
	f.written = append(f.written, rgms)
	if len(f.written) == 2 {
		return errors.New("throttled")
	}
	return nil
}

type fakeProjects bool

func (f fakeProjects) ProgramHasProjects(context.Context, string) (bool, error) { return bool(f), nil }

func newTestService(store *fakeStore, hasProjects bool) (*Service, *bytes.Buffer, func()) {
	var buf bytes.Buffer
	restore := audit.SetOutput(&buf)
	svc := NewService(store, fakeProjects(hasProjects))
	svc.newID = func() string { return "id-1" }
	return svc, &buf, restore
}

func wantErr(t *testing.T, err error, kind apperr.Kind, msg string) {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok || e.Kind != kind || e.Message != msg {
		t.Fatalf("err = %v, want kind %d %q", err, kind, msg)
	}
}

var engNight = Cohort{ID: "c1", Course: "Engenharia de Software", Shift: "NOITE", Term: "2026.2"}

func TestCreateCohortValidation(t *testing.T) {
	svc, _, restore := newTestService(&fakeStore{}, false)
	defer restore()
	cases := map[CohortInput]string{
		{Course: " ", Shift: "NOITE", Term: "1"}:                      msgCohortFieldsRequired,
		{Course: "Medicina", Shift: "NOITE", Term: "1"}:               msgInvalidCourse,
		{Course: "Engenharia de Software", Shift: "TARDE", Term: "1"}: msgInvalidShift,
	}
	for in, msg := range cases {
		_, err := svc.CreateCohort(context.Background(), in)
		wantErr(t, err, apperr.KindInvalid, msg)
	}
}

func TestCreateCohort(t *testing.T) {
	svc, buf, restore := newTestService(&fakeStore{}, false)
	defer restore()
	c, err := svc.CreateCohort(context.Background(), CohortInput{Course: " Engenharia de Software ", Shift: "NOITE", Term: " 2026.2 "})
	if err != nil || c != (Cohort{ID: "id-1", Course: "Engenharia de Software", Shift: "NOITE", Term: "2026.2"}) {
		t.Fatalf("CreateCohort = %+v, %v", c, err)
	}
	if !strings.Contains(buf.String(), `"acao":"turma.criada","resultado":"sucesso"`) {
		t.Fatalf("audit = %s", buf)
	}
}

func TestCohortStoreErrors(t *testing.T) {
	in := CohortInput{Course: engNight.Course, Shift: "MANHA", Term: engNight.Term}
	cases := []struct {
		storeErr error
		kind     apperr.Kind
		msg      string
	}{
		{errDuplicate, apperr.KindConflict, msgCohortDuplicate},
		{errStale, apperr.KindConflict, msgCohortChanged},
		{errors.New("boom"), apperr.KindInternal, "falha ao atualizar turma"},
	}
	for _, c := range cases {
		svc, _, restore := newTestService(&fakeStore{cohorts: map[string]Cohort{"c1": engNight}, err: c.storeErr}, false)
		wantErr(t, svc.UpdateCohort(context.Background(), "c1", in), c.kind, c.msg)
		restore()
	}

	svc, _, restore := newTestService(&fakeStore{}, false)
	defer restore()
	wantErr(t, svc.UpdateCohort(context.Background(), "x", in), apperr.KindNotFound, msgCohortNotFound)
	wantErr(t, svc.DeleteCohort(context.Background(), "x"), apperr.KindNotFound, msgCohortNotFound)
}

func TestUpdateCohortUnchangedSkipsStore(t *testing.T) {
	store := &fakeStore{cohorts: map[string]Cohort{"c1": engNight}, err: errors.New("must not be called")}
	svc, _, restore := newTestService(store, false)
	defer restore()
	in := CohortInput{Course: engNight.Course, Shift: engNight.Shift, Term: engNight.Term}
	if err := svc.UpdateCohort(context.Background(), "c1", in); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteCohortWithProgram(t *testing.T) {
	svc, buf, restore := newTestService(&fakeStore{cohorts: map[string]Cohort{"c1": engNight}, err: errInUse}, false)
	defer restore()
	wantErr(t, svc.DeleteCohort(context.Background(), "c1"), apperr.KindConflict, msgCohortHasProgram)
	if !strings.Contains(buf.String(), `"motivo":"turma possui programa vinculado"`) {
		t.Fatalf("audit = %s", buf)
	}
}

func TestDeleteProgramWithProjects(t *testing.T) {
	store := &fakeStore{programs: map[string]Program{"p1": {ID: "p1", CohortID: "c1"}}, err: errors.New("must not be called")}
	svc, _, restore := newTestService(store, true)
	defer restore()
	wantErr(t, svc.DeleteProgram(context.Background(), "p1"), apperr.KindConflict, msgProgramHasProjects)
}

func TestEnroll(t *testing.T) {
	store := &fakeStore{cohorts: map[string]Cohort{"c1": engNight}}
	svc, buf, restore := newTestService(store, false)
	defer restore()

	rgms := []string{" 1 ", "1", "", "12a"}
	for i := 2; i <= 30; i++ {
		rgms = append(rgms, fmt.Sprint(i))
	}
	result, err := svc.Enroll(context.Background(), EnrollmentRequest{RGMs: rgms, CohortID: " c1 "})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.written) != 2 || len(store.written[0]) != enrollmentBatch || len(store.written[1]) != 5 {
		t.Fatalf("batches = %v", store.written)
	}
	if result.Processed != 25 || len(result.Failures) != 6 || result.Failures[0] != (EnrollmentFailure{RGM: "12a", Error: msgRGMDigitsOnly}) {
		t.Fatalf("result = %+v", result)
	}
	if want := []string{"26", "27", "28", "29", "30"}; !reflect.DeepEqual(store.written[1], want) || result.Failures[1].Error != "throttled" {
		t.Fatalf("second batch = %v, failures = %+v", store.written[1], result.Failures)
	}
	if !strings.Contains(buf.String(), `"motivo":"6 de 31 RGMs falharam"`) {
		t.Fatalf("audit = %s", buf)
	}
}

func TestEnrollValidation(t *testing.T) {
	svc, _, restore := newTestService(&fakeStore{cohorts: map[string]Cohort{"c1": engNight}}, false)
	defer restore()
	ctx := context.Background()

	_, err := svc.Enroll(ctx, EnrollmentRequest{RGMs: []string{"1"}})
	wantErr(t, err, apperr.KindInvalid, msgEnrollCohortRequired)
	_, err = svc.Enroll(ctx, EnrollmentRequest{RGMs: []string{"1"}, CohortID: "x"})
	wantErr(t, err, apperr.KindNotFound, msgCohortNotFound)
	_, err = svc.Enroll(ctx, EnrollmentRequest{RGMs: []string{" "}, CohortID: "c1"})
	wantErr(t, err, apperr.KindInvalid, msgRGMRequired)
	_, err = svc.Unenroll(ctx, EnrollmentRequest{})
	wantErr(t, err, apperr.KindInvalid, msgRGMRequired)
}
