package academic

import (
	"context"
	"errors"
)

// Store errors: the service turns them into the message of each operation.
var (
	errNotFound  = errors.New("not found")
	errDuplicate = errors.New("uniqueness lock held by another item")
	errStale     = errors.New("item changed since it was read")
	errInUse     = errors.New("item still referenced")
	errNoCohort  = errors.New("cohort does not exist")
	errNoCourse  = errors.New("course does not exist")
)

type Store interface {
	CreateCourse(ctx context.Context, c Course) error
	GetCourse(ctx context.Context, id string) (Course, bool, error)
	ListCourses(ctx context.Context) ([]Course, error)
	// CourseByName ignores case and returns the course with its stored name.
	CourseByName(ctx context.Context, name string) (Course, bool, error)
	UpdateCourse(ctx context.Context, current, next Course) error
	// DeleteCourse returns errInUse while the course still has cohorts.
	DeleteCourse(ctx context.Context, c Course) error

	CreateCohort(ctx context.Context, c Cohort) error
	GetCohort(ctx context.Context, id string) (Cohort, bool, error)
	ListCohorts(ctx context.Context) ([]Cohort, error)
	CohortExists(ctx context.Context, id string) (bool, error)
	UpdateCohort(ctx context.Context, current, next Cohort) error
	DeleteCohort(ctx context.Context, c Cohort) error

	CreateProgram(ctx context.Context, p Program) error
	GetProgram(ctx context.Context, id string) (Program, bool, error)
	ListPrograms(ctx context.Context) ([]Program, error)
	ProgramExists(ctx context.Context, id string) (bool, error)
	ProgramOfCohort(ctx context.Context, cohortID string) (string, bool, error)
	MoveProgram(ctx context.Context, current Program, cohortID string) error
	DeleteProgram(ctx context.Context, p Program) error

	ListEnrollments(ctx context.Context) ([]Enrollment, error)
	PutEnrollments(ctx context.Context, rgms []string, cohortID string) error
	DeleteEnrollments(ctx context.Context, rgms []string) error
	EnrollmentExists(ctx context.Context, rgm string) (bool, error)
	CohortOfStudent(ctx context.Context, rgm string) (string, error)
	CohortsOfStudents(ctx context.Context, rgms []string) (map[string]string, error)
}

// Projects belong to the groups module; academic only asks whether a
// program still has any.
type Projects interface {
	ProgramHasProjects(ctx context.Context, programID string) (bool, error)
}
