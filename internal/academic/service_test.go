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
	courses  map[string]Course
	cohorts  map[string]Cohort
	programs map[string]Program
	err      error
	written  [][]string
	// cohorts handed to UpdateCourse, to check they follow the rename
	renamed []Cohort
}

func (f *fakeStore) CourseByName(_ context.Context, name string) (Course, bool, error) {
	for _, c := range f.courses {
		if strings.EqualFold(c.Name, name) {
			return c, true, nil
		}
	}
	return Course{}, false, nil
}
func (f *fakeStore) GetCourse(_ context.Context, id string) (Course, bool, error) {
	c, ok := f.courses[id]
	return c, ok, nil
}
func (f *fakeStore) ListCourses(context.Context) ([]Course, error) {
	var out []Course
	for _, c := range f.courses {
		out = append(out, c)
	}
	return out, nil
}
func (f *fakeStore) CreateCourse(context.Context, Course) error { return f.err }
func (f *fakeStore) UpdateCourse(_ context.Context, _, _ Course, cohorts []Cohort) error {
	f.renamed = cohorts
	return f.err
}
func (f *fakeStore) DeleteCourse(context.Context, Course) error { return f.err }
func (f *fakeStore) ListCohorts(context.Context) ([]Cohort, error) {
	var out []Cohort
	for _, c := range f.cohorts {
		out = append(out, c)
	}
	return out, nil
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

var (
	engNight  = Cohort{ID: "c1", Course: "Engenharia de Software", Shift: "NOITE", Term: "2026.2"}
	engCourse = map[string]Course{"k1": {ID: "k1", Name: "Engenharia de Software"}}
)

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
	svc, buf, restore := newTestService(&fakeStore{courses: engCourse}, false)
	defer restore()
	// the cohort takes the registered spelling of the course, whatever was typed
	c, err := svc.CreateCohort(context.Background(), CohortInput{Course: " engenharia de software ", Shift: "NOITE", Term: " 2026.2 "})
	if err != nil || c != (Cohort{ID: "id-1", Course: "Engenharia de Software", Shift: "NOITE", Term: "2026.2"}) {
		t.Fatalf("CreateCohort = %+v, %v", c, err)
	}
	if !strings.Contains(buf.String(), `"acao":"turma.criada","resultado":"sucesso"`) {
		t.Fatalf("audit = %s", buf)
	}
}

func TestCreateCohortCourseGone(t *testing.T) {
	svc, _, restore := newTestService(&fakeStore{courses: engCourse, err: errNoCourse}, false)
	defer restore()
	_, err := svc.CreateCohort(context.Background(), CohortInput{Course: "Engenharia de Software", Shift: "NOITE", Term: "2026.2"})
	wantErr(t, err, apperr.KindInvalid, msgInvalidCourse)
}

func TestCourseValidation(t *testing.T) {
	svc, _, restore := newTestService(&fakeStore{}, false)
	defer restore()
	_, err := svc.CreateCourse(context.Background(), CourseInput{Name: "  "})
	wantErr(t, err, apperr.KindInvalid, msgCourseNameRequired)
	_, err = svc.CreateCourse(context.Background(), CourseInput{Name: strings.Repeat("a", maxCourseNameLen+1)})
	wantErr(t, err, apperr.KindInvalid, msgCourseNameTooLong)
}

func TestCreateCourse(t *testing.T) {
	svc, buf, restore := newTestService(&fakeStore{}, false)
	defer restore()
	c, err := svc.CreateCourse(context.Background(), CourseInput{Name: "  Ciência   da Computação "})
	if err != nil || c != (Course{ID: "id-1", Name: "Ciência da Computação"}) {
		t.Fatalf("CreateCourse = %+v, %v", c, err)
	}
	if !strings.Contains(buf.String(), `"acao":"curso.criado","resultado":"sucesso"`) {
		t.Fatalf("audit = %s", buf)
	}

	svc, _, restore = newTestService(&fakeStore{err: errDuplicate}, false)
	defer restore()
	_, err = svc.CreateCourse(context.Background(), CourseInput{Name: "X"})
	wantErr(t, err, apperr.KindConflict, msgCourseDuplicate)
}

func TestUpdateCourseCarriesCohorts(t *testing.T) {
	store := &fakeStore{courses: engCourse, cohorts: map[string]Cohort{
		"c1": engNight,
		"c2": {ID: "c2", Course: "Outro", Shift: "NOITE", Term: "2026.2"},
	}}
	svc, _, restore := newTestService(store, false)
	defer restore()

	if err := svc.UpdateCourse(context.Background(), "k1", CourseInput{Name: "Engenharia de Software e IA"}); err != nil {
		t.Fatal(err)
	}
	if len(store.renamed) != 1 || store.renamed[0].ID != "c1" {
		t.Fatalf("cohorts handed to the store = %+v, want only c1", store.renamed)
	}
}

func TestUpdateCourseErrors(t *testing.T) {
	ctx := context.Background()
	in := CourseInput{Name: "Novo nome"}
	cases := []struct {
		storeErr error
		kind     apperr.Kind
		msg      string
	}{
		{errDuplicate, apperr.KindConflict, msgCourseDuplicate},
		{errStale, apperr.KindConflict, msgCourseChanged},
		{errTooMany, apperr.KindConflict, msgCourseTooManyCohorts},
		{errors.New("boom"), apperr.KindInternal, "falha ao atualizar curso"},
	}
	for _, c := range cases {
		svc, _, restore := newTestService(&fakeStore{courses: engCourse, err: c.storeErr}, false)
		wantErr(t, svc.UpdateCourse(ctx, "k1", in), c.kind, c.msg)
		restore()
	}

	svc, _, restore := newTestService(&fakeStore{courses: engCourse}, false)
	defer restore()
	wantErr(t, svc.UpdateCourse(ctx, "x", in), apperr.KindNotFound, msgCourseUnknown)
	wantErr(t, svc.UpdateCourse(ctx, "k1", CourseInput{}), apperr.KindInvalid, msgCourseNameRequired)
}

func TestUpdateCourseUnchangedSkipsStore(t *testing.T) {
	store := &fakeStore{courses: engCourse, err: errors.New("must not be called")}
	svc, _, restore := newTestService(store, false)
	defer restore()
	if err := svc.UpdateCourse(context.Background(), "k1", CourseInput{Name: " Engenharia de Software "}); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteCourse(t *testing.T) {
	ctx := context.Background()

	svc, buf, restore := newTestService(&fakeStore{courses: engCourse, cohorts: map[string]Cohort{"c1": engNight}, err: errors.New("must not be called")}, false)
	wantErr(t, svc.DeleteCourse(ctx, "k1"), apperr.KindConflict, msgCourseHasCohorts)
	if !strings.Contains(buf.String(), `"motivo":"curso possui turmas"`) {
		t.Fatalf("audit = %s", buf)
	}
	restore()

	svc, _, restore = newTestService(&fakeStore{courses: engCourse}, false)
	if err := svc.DeleteCourse(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
	wantErr(t, svc.DeleteCourse(ctx, "x"), apperr.KindNotFound, msgCourseUnknown)
	restore()
}

func TestListCoursesSorted(t *testing.T) {
	store := &fakeStore{courses: map[string]Course{
		"a": {ID: "a", Name: "sistemas de Informação"},
		"b": {ID: "b", Name: "Engenharia de Software"},
	}}
	svc, _, restore := newTestService(store, false)
	defer restore()
	courses, err := svc.ListCourses(context.Background())
	if err != nil || len(courses) != 2 || courses[0].ID != "b" {
		t.Fatalf("ListCourses = %+v, %v", courses, err)
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
		svc, _, restore := newTestService(&fakeStore{courses: engCourse, cohorts: map[string]Cohort{"c1": engNight}, err: c.storeErr}, false)
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
