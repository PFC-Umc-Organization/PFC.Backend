package academic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

// Items (single table):
//
//	course          PK=COURSE#<id>                          SK=PROFILE      GSI1PK=COURSES     GSI1SK=COURSE#<id>  nome, turmas
//	course name     PK=CURSO_NOME#<name, lower case>        SK=CURSO_NOME   cursoId, nome
//	cohort          PK=CURSO#<id>                           SK=PROFILE      GSI1PK=TURMAS      GSI1SK=CURSO#<id>   graduacaoId, turno, periodo
//	cohort lock     PK=TURMA_UNICA#<course id>#<shift>#<term> SK=TURMA_UNICA  cursoId
//	cohort program  PK=CURSO#<id>                         SK=PROGRAMA     programaId
//	program         PK=PROGRAM#<id>                       SK=PROFILE      GSI1PK=PROGRAMAS   GSI1SK=PROGRAM#<id>  cursoId
//	enrollment      PK=STUDENT#<rgm>                      SK=PROFILE      GSI1PK=MATRICULAS#<turmaId> GSI1SK=STUDENT#<rgm> status, turmaId
//
// Despite the prefix, CURSO# is a cohort and COURSE# a course: CURSO# predates
// courses and its keys cannot change. A cohort points to its course by id
// (graduacaoId) and the course counts its cohorts (turmas) in the same
// transactions that create, move and delete cohorts, so a course with cohorts
// cannot be deleted. Cohorts written before courses existed carry only the
// course name (nome) and the lock keyed by it; cmd/migrar moves them over.
//
// The lock and the cohort-program link exist so that a transaction rejects
// a second cohort with the same course/shift/term and a second program per
// cohort. The Pre Sign-up Lambda reads the enrollment item by key, so its
// PK and SK cannot change.
const (
	coursePrefix     = "COURSE#"
	courseNamePrefix = "CURSO_NOME#"
	courseNameSK     = "CURSO_NOME"
	cohortPrefix     = "CURSO#"
	programPrefix    = "PROGRAM#"
	enrollmentPrefix = "STUDENT#"
	profileSK        = "PROFILE"

	coursesPartition  = "COURSES"
	cohortsPartition  = "TURMAS"
	programsPartition = "PROGRAMAS"

	enrollmentStatusActive = "ACTIVE"
)

type DynamoStore struct {
	table *dynamo.Table
}

func NewDynamoStore(t *dynamo.Table) *DynamoStore { return &DynamoStore{table: t} }

type courseItem struct {
	PK      string `dynamodbav:"PK"`
	SK      string `dynamodbav:"SK"`
	GSI1PK  string `dynamodbav:"GSI1PK"`
	GSI1SK  string `dynamodbav:"GSI1SK"`
	Name    string `dynamodbav:"nome"`
	Cohorts int    `dynamodbav:"turmas"`
}

type cohortItem struct {
	PK       string `dynamodbav:"PK"`
	SK       string `dynamodbav:"SK"`
	GSI1PK   string `dynamodbav:"GSI1PK"`
	GSI1SK   string `dynamodbav:"GSI1SK"`
	CourseID string `dynamodbav:"graduacaoId,omitempty"`
	Course   string `dynamodbav:"nome,omitempty"`
	Shift    string `dynamodbav:"turno"`
	Term     string `dynamodbav:"periodo"`
}

func (it cohortItem) cohort() Cohort {
	return Cohort{ID: strings.TrimPrefix(it.PK, cohortPrefix), CourseID: it.CourseID, Course: it.Course, Shift: it.Shift, Term: it.Term}
}

type programItem struct {
	PK       string `dynamodbav:"PK"`
	SK       string `dynamodbav:"SK"`
	GSI1PK   string `dynamodbav:"GSI1PK"`
	GSI1SK   string `dynamodbav:"GSI1SK"`
	CohortID string `dynamodbav:"cursoId"`
}

func (it programItem) program() Program {
	return Program{ID: strings.TrimPrefix(it.PK, programPrefix), CohortID: it.CohortID}
}

type enrollmentItem struct {
	PK       string `dynamodbav:"PK"`
	Status   string `dynamodbav:"status"`
	CohortID string `dynamodbav:"turmaId"`
}

func cohortKey(id string) dynamo.Item      { return dynamo.Key(cohortPrefix+id, profileSK) }
func programKey(id string) dynamo.Item     { return dynamo.Key(programPrefix+id, profileSK) }
func enrollmentKey(rgm string) dynamo.Item { return dynamo.Key(enrollmentPrefix+rgm, profileSK) }
func cohortProgramKey(cohortID string) dynamo.Item {
	return dynamo.Key(cohortPrefix+cohortID, "PROGRAMA")
}

func cohortLock(c Cohort) dynamo.Item {
	return dynamo.Key(CohortLockPK(c.CourseID, c.Shift, c.Term), "TURMA_UNICA")
}

// CohortLockPK is shared with cmd/migrar.
func CohortLockPK(courseID, shift, term string) string {
	return fmt.Sprintf("TURMA_UNICA#%s#%s#%s", courseID, shift, term)
}

// currentLock is the lock a stored cohort holds: keyed by course name when
// the cohort predates courses.
func currentLock(c Cohort) dynamo.Item {
	if c.CourseID == "" {
		return dynamo.Key(fmt.Sprintf("TURMA_UNICA#%s#%s#%s", c.Course, c.Shift, c.Term), "TURMA_UNICA")
	}
	return cohortLock(c)
}

func (s *DynamoStore) countCohort(courseID string, delta int) types.TransactWriteItem {
	return s.table.Update(courseKey(courseID), "SET turmas = if_not_exists(turmas, :zero) + :delta", present,
		dynamo.Item{":zero": dynamo.N(0), ":delta": dynamo.N(delta)})
}

func ownedLock(c Cohort) dynamo.Item {
	lock := cohortLock(c)
	lock["cursoId"] = dynamo.S(c.ID)
	return lock
}

func cohortProgramLink(cohortID, programID string) dynamo.Item {
	link := cohortProgramKey(cohortID)
	link["programaId"] = dynamo.S(programID)
	return link
}

const (
	absent         = "attribute_not_exists(PK)"
	present        = "attribute_exists(PK)"
	ownedByCohort  = "attribute_not_exists(PK) OR cursoId = :id"
	ownedByProgram = "attribute_not_exists(PK) OR programaId = :id"
)

func (it courseItem) course() Course {
	return Course{ID: strings.TrimPrefix(it.PK, coursePrefix), Name: it.Name}
}

func courseKey(id string) dynamo.Item { return dynamo.Key(coursePrefix+id, profileSK) }

// courseNameKey is the uniqueness lock of a course name, ignoring case.
func courseNameKey(name string) dynamo.Item {
	return dynamo.Key(courseNamePrefix+strings.ToLower(name), courseNameSK)
}

func ownedCourseName(c Course) dynamo.Item {
	lock := courseNameKey(c.Name)
	lock["cursoId"] = dynamo.S(c.ID)
	lock["nome"] = dynamo.S(c.Name)
	return lock
}

func (s *DynamoStore) CreateCourse(ctx context.Context, c Course) error {
	item, err := attributevalue.MarshalMap(courseItem{
		PK: coursePrefix + c.ID, SK: profileSK, GSI1PK: coursesPartition, GSI1SK: coursePrefix + c.ID, Name: c.Name,
	})
	if err != nil {
		return err
	}
	err = s.table.Transact(ctx,
		s.table.Put(item, absent, nil),
		s.table.Put(ownedCourseName(c), absent, nil),
	)
	if dynamo.FailedAt(err, 1) {
		return errDuplicate
	}
	return err
}

func (s *DynamoStore) GetCourse(ctx context.Context, id string) (Course, bool, error) {
	it, found, err := dynamo.Get[courseItem](ctx, s.table, courseKey(id))
	return it.course(), found, err
}

func (s *DynamoStore) ListCourses(ctx context.Context) ([]Course, error) {
	items, err := dynamo.QueryIndex[courseItem](ctx, s.table, coursesPartition)
	return mapItems(items, courseItem.course), err
}

func (s *DynamoStore) CourseByName(ctx context.Context, name string) (Course, bool, error) {
	lock, found, err := dynamo.Get[struct {
		ID   string `dynamodbav:"cursoId"`
		Name string `dynamodbav:"nome"`
	}](ctx, s.table, courseNameKey(name), "cursoId", "nome")
	return Course{ID: lock.ID, Name: lock.Name}, found, err
}

// UpdateCourse renames the course and swaps its name lock.
func (s *DynamoStore) UpdateCourse(ctx context.Context, current, next Course) error {
	rename := s.table.Update(courseKey(current.ID), "SET nome = :n", "nome = :old",
		dynamo.Item{":n": dynamo.S(next.Name), ":old": dynamo.S(current.Name)})
	// A case-only change keeps the lock key, and a transaction cannot delete
	// and put the same item: the lock is updated in place.
	if strings.EqualFold(current.Name, next.Name) {
		return s.staleOnFailure(s.table.Transact(ctx, rename,
			s.table.Update(courseNameKey(current.Name), "SET nome = :n", "cursoId = :id",
				dynamo.Item{":n": dynamo.S(next.Name), ":id": dynamo.S(current.ID)})))
	}
	err := s.table.Transact(ctx, rename,
		s.table.Delete(courseNameKey(current.Name), "cursoId = :id", dynamo.Item{":id": dynamo.S(current.ID)}),
		s.table.Put(ownedCourseName(next), absent, nil),
	)
	if dynamo.FailedAt(err, 2) {
		return errDuplicate
	}
	return s.staleOnFailure(err)
}

func (s *DynamoStore) staleOnFailure(err error) error {
	var rejected *dynamo.ConditionError
	if errors.As(err, &rejected) {
		return errStale
	}
	return err
}

func (s *DynamoStore) DeleteCourse(ctx context.Context, c Course) error {
	err := s.table.Transact(ctx,
		s.table.Delete(courseKey(c.ID), "attribute_exists(PK) AND (attribute_not_exists(turmas) OR turmas = :zero)",
			dynamo.Item{":zero": dynamo.N(0)}),
		s.table.Delete(courseNameKey(c.Name), "cursoId = :id", dynamo.Item{":id": dynamo.S(c.ID)}),
	)
	switch {
	case dynamo.FailedAt(err, 0):
		if exists, _ := s.table.Exists(ctx, courseKey(c.ID)); !exists {
			return errNotFound
		}
		return errInUse
	case dynamo.FailedAt(err, 1):
		return errStale
	}
	return err
}

func (s *DynamoStore) CreateCohort(ctx context.Context, c Cohort) error {
	item, err := attributevalue.MarshalMap(cohortItem{
		PK: cohortPrefix + c.ID, SK: profileSK, GSI1PK: cohortsPartition, GSI1SK: cohortPrefix + c.ID,
		CourseID: c.CourseID, Shift: c.Shift, Term: c.Term,
	})
	if err != nil {
		return err
	}
	err = s.table.Transact(ctx,
		s.table.Put(item, absent, nil),
		s.table.Put(ownedLock(c), absent, nil),
		s.countCohort(c.CourseID, 1),
	)
	switch {
	case dynamo.FailedAt(err, 2):
		return errNoCourse
	case dynamo.FailedAt(err, 1):
		return errDuplicate
	}
	return err
}

func (s *DynamoStore) GetCohort(ctx context.Context, id string) (Cohort, bool, error) {
	it, found, err := dynamo.Get[cohortItem](ctx, s.table, cohortKey(id))
	return it.cohort(), found, err
}

func (s *DynamoStore) ListCohorts(ctx context.Context) ([]Cohort, error) {
	items, err := dynamo.QueryIndex[cohortItem](ctx, s.table, cohortsPartition)
	return mapItems(items, cohortItem.cohort), err
}

func (s *DynamoStore) CohortExists(ctx context.Context, id string) (bool, error) {
	return s.table.Exists(ctx, cohortKey(id))
}

// UpdateCohort swaps the lock and, when the course changes, moves the count
// to the new course; the condition on the cohort rejects a concurrent edit.
func (s *DynamoStore) UpdateCohort(ctx context.Context, current, next Cohort) error {
	cond := "graduacaoId = :vc AND turno = :vt AND periodo = :vp"
	values := dynamo.Item{
		":c": dynamo.S(next.CourseID), ":t": dynamo.S(next.Shift), ":p": dynamo.S(next.Term),
		":vc": dynamo.S(current.CourseID), ":vt": dynamo.S(current.Shift), ":vp": dynamo.S(current.Term),
	}
	if current.CourseID == "" {
		cond = "attribute_not_exists(graduacaoId) AND turno = :vt AND periodo = :vp"
		delete(values, ":vc")
	}
	ops := []types.TransactWriteItem{
		s.table.Update(cohortKey(current.ID), "SET graduacaoId = :c, turno = :t, periodo = :p REMOVE nome", cond, values),
		s.table.Delete(currentLock(current), ownedByCohort, dynamo.Item{":id": dynamo.S(current.ID)}),
		s.table.Put(ownedLock(next), absent, nil),
	}
	if current.CourseID != next.CourseID {
		ops = append(ops, s.countCohort(next.CourseID, 1))
		if current.CourseID != "" {
			ops = append(ops, s.countCohort(current.CourseID, -1))
		}
	}
	err := s.table.Transact(ctx, ops...)
	switch {
	case dynamo.FailedAt(err, 3):
		return errNoCourse
	case dynamo.FailedAt(err, 2):
		return errDuplicate
	case dynamo.FailedAt(err, 0), dynamo.FailedAt(err, 1), dynamo.FailedAt(err, 4):
		return errStale
	}
	return err
}

func (s *DynamoStore) DeleteCohort(ctx context.Context, c Cohort) error {
	ops := []types.TransactWriteItem{
		s.table.Delete(cohortKey(c.ID), present, nil),
		s.table.Delete(currentLock(c), ownedByCohort, dynamo.Item{":id": dynamo.S(c.ID)}),
		s.table.Check(cohortProgramKey(c.ID), absent, nil),
	}
	if c.CourseID != "" {
		ops = append(ops, s.countCohort(c.CourseID, -1))
	}
	err := s.table.Transact(ctx, ops...)
	switch {
	case dynamo.FailedAt(err, 2):
		return errInUse
	case dynamo.FailedAt(err, 0):
		return errNotFound
	case dynamo.FailedAt(err, 1), dynamo.FailedAt(err, 3):
		return errStale
	}
	return err
}

func (s *DynamoStore) CreateProgram(ctx context.Context, p Program) error {
	item, err := attributevalue.MarshalMap(programItem{
		PK: programPrefix + p.ID, SK: profileSK, GSI1PK: programsPartition, GSI1SK: programPrefix + p.ID, CohortID: p.CohortID,
	})
	if err != nil {
		return err
	}
	err = s.table.Transact(ctx,
		s.table.Check(cohortKey(p.CohortID), present, nil),
		s.table.Put(cohortProgramLink(p.CohortID, p.ID), absent, nil),
		s.table.Put(item, absent, nil),
	)
	switch {
	case dynamo.FailedAt(err, 0):
		return errNoCohort
	case dynamo.FailedAt(err, 1):
		return errDuplicate
	}
	return err
}

func (s *DynamoStore) GetProgram(ctx context.Context, id string) (Program, bool, error) {
	it, found, err := dynamo.Get[programItem](ctx, s.table, programKey(id))
	return it.program(), found, err
}

func (s *DynamoStore) ListPrograms(ctx context.Context) ([]Program, error) {
	items, err := dynamo.QueryIndex[programItem](ctx, s.table, programsPartition)
	return mapItems(items, programItem.program), err
}

func (s *DynamoStore) ProgramExists(ctx context.Context, id string) (bool, error) {
	return s.table.Exists(ctx, programKey(id))
}

func (s *DynamoStore) ProgramOfCohort(ctx context.Context, cohortID string) (string, bool, error) {
	link, found, err := dynamo.Get[struct {
		ProgramID string `dynamodbav:"programaId"`
	}](ctx, s.table, cohortProgramKey(cohortID), "programaId")
	return link.ProgramID, found, err
}

func (s *DynamoStore) MoveProgram(ctx context.Context, current Program, cohortID string) error {
	err := s.table.Transact(ctx,
		s.table.Update(programKey(current.ID), "SET cursoId = :new", "cursoId = :old",
			dynamo.Item{":new": dynamo.S(cohortID), ":old": dynamo.S(current.CohortID)}),
		s.table.Delete(cohortProgramKey(current.CohortID), ownedByProgram, dynamo.Item{":id": dynamo.S(current.ID)}),
		s.table.Check(cohortKey(cohortID), present, nil),
		s.table.Put(cohortProgramLink(cohortID, current.ID), absent, nil),
	)
	switch {
	case dynamo.FailedAt(err, 2):
		return errNoCohort
	case dynamo.FailedAt(err, 3):
		return errDuplicate
	case dynamo.FailedAt(err, 0), dynamo.FailedAt(err, 1):
		return errStale
	}
	return err
}

func (s *DynamoStore) DeleteProgram(ctx context.Context, p Program) error {
	err := s.table.Transact(ctx,
		s.table.Delete(programKey(p.ID), present, nil),
		s.table.Delete(cohortProgramKey(p.CohortID), ownedByProgram, dynamo.Item{":id": dynamo.S(p.ID)}),
	)
	switch {
	case dynamo.FailedAt(err, 0):
		return errNotFound
	case dynamo.FailedAt(err, 1):
		return errStale
	}
	return err
}

// EnrollmentsPartition is the GSI1 partition of a cohort's enrollments.
func EnrollmentsPartition(cohortID string) string { return "MATRICULAS#" + cohortID }

func (s *DynamoStore) ListEnrollments(ctx context.Context, cohortID string) ([]Enrollment, error) {
	items, err := dynamo.QueryIndex[enrollmentItem](ctx, s.table, EnrollmentsPartition(cohortID))
	return mapItems(items, func(it enrollmentItem) Enrollment {
		status := EnrollmentInactive
		if it.Status == enrollmentStatusActive {
			status = EnrollmentActive
		}
		return Enrollment{RGM: strings.TrimPrefix(it.PK, enrollmentPrefix), Status: status, CohortID: it.CohortID}
	}), err
}

func (s *DynamoStore) PutEnrollments(ctx context.Context, rgms []string, cohortID string) error {
	writes := make([]types.WriteRequest, 0, len(rgms))
	for _, rgm := range rgms {
		writes = append(writes, types.WriteRequest{PutRequest: &types.PutRequest{Item: dynamo.Item{
			"PK":      dynamo.S(enrollmentPrefix + rgm),
			"SK":      dynamo.S(profileSK),
			"GSI1PK":  dynamo.S(EnrollmentsPartition(cohortID)),
			"GSI1SK":  dynamo.S(enrollmentPrefix + rgm),
			"status":  dynamo.S(enrollmentStatusActive),
			"turmaId": dynamo.S(cohortID),
		}}})
	}
	return s.table.BatchWrite(ctx, writes)
}

func (s *DynamoStore) DeleteEnrollments(ctx context.Context, rgms []string) error {
	keys := make([]dynamo.Item, 0, len(rgms))
	for _, rgm := range rgms {
		keys = append(keys, enrollmentKey(rgm))
	}
	return s.table.DeleteKeys(ctx, keys)
}

func (s *DynamoStore) EnrollmentExists(ctx context.Context, rgm string) (bool, error) {
	return s.table.Exists(ctx, enrollmentKey(rgm))
}

func (s *DynamoStore) CohortOfStudent(ctx context.Context, rgm string) (string, error) {
	it, _, err := dynamo.Get[enrollmentItem](ctx, s.table, enrollmentKey(rgm), "turmaId")
	return it.CohortID, err
}

func (s *DynamoStore) CohortsOfStudents(ctx context.Context, rgms []string) (map[string]string, error) {
	keys := make([]dynamo.Item, 0, len(rgms))
	for _, rgm := range rgms {
		keys = append(keys, enrollmentKey(rgm))
	}
	items, err := s.table.BatchGet(ctx, keys, "PK", "turmaId")
	if err != nil {
		return nil, err
	}
	cohorts := make(map[string]string, len(items))
	for _, it := range items {
		if c := dynamo.String(it, "turmaId"); c != "" {
			cohorts[strings.TrimPrefix(dynamo.String(it, "PK"), enrollmentPrefix)] = c
		}
	}
	return cohorts, nil
}

func mapItems[I, T any](items []I, f func(I) T) []T {
	out := make([]T, 0, len(items))
	for _, it := range items {
		out = append(out, f(it))
	}
	return out
}
