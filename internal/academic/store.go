package academic

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

// Items (single table):
//
//	cohort          PK=CURSO#<id>                         SK=PROFILE      GSI1PK=TURMAS      GSI1SK=CURSO#<id>
//	cohort lock     PK=TURMA_UNICA#<course>#<shift>#<term> SK=TURMA_UNICA  cursoId
//	cohort program  PK=CURSO#<id>                         SK=PROGRAMA     programaId
//	program         PK=PROGRAM#<id>                       SK=PROFILE      GSI1PK=PROGRAMAS   GSI1SK=PROGRAM#<id>  cursoId
//	enrollment      PK=STUDENT#<rgm>                      SK=PROFILE      GSI1PK=MATRICULAS  GSI1SK=STUDENT#<rgm> status, turmaId
//
// The lock and the cohort-program link exist so that a transaction rejects
// a second cohort with the same course/shift/term and a second program per
// cohort. The Pre Sign-up Lambda reads the enrollment item by key, so its
// PK and SK cannot change.
const (
	cohortPrefix     = "CURSO#"
	programPrefix    = "PROGRAM#"
	enrollmentPrefix = "STUDENT#"
	profileSK        = "PROFILE"

	cohortsPartition     = "TURMAS"
	programsPartition    = "PROGRAMAS"
	enrollmentsPartition = "MATRICULAS"

	enrollmentStatusActive = "ACTIVE"
)

type DynamoStore struct {
	table *dynamo.Table
}

func NewDynamoStore(t *dynamo.Table) *DynamoStore { return &DynamoStore{table: t} }

type cohortItem struct {
	PK     string `dynamodbav:"PK"`
	SK     string `dynamodbav:"SK"`
	GSI1PK string `dynamodbav:"GSI1PK"`
	GSI1SK string `dynamodbav:"GSI1SK"`
	Course string `dynamodbav:"nome"`
	Shift  string `dynamodbav:"turno"`
	Term   string `dynamodbav:"periodo"`
}

func (it cohortItem) cohort() Cohort {
	return Cohort{ID: strings.TrimPrefix(it.PK, cohortPrefix), Course: it.Course, Shift: it.Shift, Term: it.Term}
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
	return dynamo.Key(fmt.Sprintf("TURMA_UNICA#%s#%s#%s", c.Course, c.Shift, c.Term), "TURMA_UNICA")
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

func (s *DynamoStore) CreateCohort(ctx context.Context, c Cohort) error {
	item, err := attributevalue.MarshalMap(cohortItem{
		PK: cohortPrefix + c.ID, SK: profileSK, GSI1PK: cohortsPartition, GSI1SK: cohortPrefix + c.ID,
		Course: c.Course, Shift: c.Shift, Term: c.Term,
	})
	if err != nil {
		return err
	}
	err = s.table.Transact(ctx,
		s.table.Put(item, absent, nil),
		s.table.Put(ownedLock(c), absent, nil),
	)
	if dynamo.FailedAt(err, 1) {
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

// UpdateCohort swaps the lock when the combination changes; the condition
// on the cohort rejects a concurrent edit.
func (s *DynamoStore) UpdateCohort(ctx context.Context, current, next Cohort) error {
	err := s.table.Transact(ctx,
		s.table.Update(cohortKey(current.ID),
			"SET nome = :n, turno = :t, periodo = :p",
			"nome = :vn AND turno = :vt AND periodo = :vp",
			dynamo.Item{
				":n": dynamo.S(next.Course), ":t": dynamo.S(next.Shift), ":p": dynamo.S(next.Term),
				":vn": dynamo.S(current.Course), ":vt": dynamo.S(current.Shift), ":vp": dynamo.S(current.Term),
			}),
		s.table.Delete(cohortLock(current), ownedByCohort, dynamo.Item{":id": dynamo.S(current.ID)}),
		s.table.Put(ownedLock(next), absent, nil),
	)
	switch {
	case dynamo.FailedAt(err, 2):
		return errDuplicate
	case dynamo.FailedAt(err, 0), dynamo.FailedAt(err, 1):
		return errStale
	}
	return err
}

func (s *DynamoStore) DeleteCohort(ctx context.Context, c Cohort) error {
	err := s.table.Transact(ctx,
		s.table.Delete(cohortKey(c.ID), present, nil),
		s.table.Delete(cohortLock(c), ownedByCohort, dynamo.Item{":id": dynamo.S(c.ID)}),
		s.table.Check(cohortProgramKey(c.ID), absent, nil),
	)
	switch {
	case dynamo.FailedAt(err, 2):
		return errInUse
	case dynamo.FailedAt(err, 0):
		return errNotFound
	case dynamo.FailedAt(err, 1):
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

func (s *DynamoStore) ListEnrollments(ctx context.Context) ([]Enrollment, error) {
	items, err := dynamo.QueryIndex[enrollmentItem](ctx, s.table, enrollmentsPartition)
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
			"GSI1PK":  dynamo.S(enrollmentsPartition),
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
