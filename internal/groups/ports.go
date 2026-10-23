package groups

import (
	"context"
	"errors"
)

var (
	errNotFound    = errors.New("not found")
	errDuplicate   = errors.New("already exists")
	errStale       = errors.New("item changed since it was read")
	errNotMember   = errors.New("rgm is not a member")
	errMemberTaken = errors.New("rgm already belongs to a project")
)

type Store interface {
	CreateProject(ctx context.Context, p Project) error
	GetProject(ctx context.Context, id string) (Project, bool, error)
	ProjectsOfProgram(ctx context.Context, programID string) ([]Project, error)
	ProjectOfMember(ctx context.Context, rgm string) (Project, bool, error)
	ProjectsOfMembers(ctx context.Context, rgms []string) (map[string]string, error)
	UpdateDetails(ctx context.Context, id, name, description string) error
	SetAdvisor(ctx context.Context, id, advisorID string) error
	ClearAdvisor(ctx context.Context, id string) error
	AddMember(ctx context.Context, id, rgm string, current []string) error
	RemoveMember(ctx context.Context, id, rgm string) error
	DeleteProject(ctx context.Context, id string) error

	CreateInvitation(ctx context.Context, inv Invitation) error
	InvitationExists(ctx context.Context, projectID, rgm string) (bool, error)
	InvitationsOfProject(ctx context.Context, projectID string) ([]Invitation, error)
	InvitationsOfStudent(ctx context.Context, rgm string) ([]Invitation, error)
	DeleteInvitation(ctx context.Context, projectID, rgm string) error
}

type Academic interface {
	ProgramExists(ctx context.Context, id string) (bool, error)
	CohortOfProgram(ctx context.Context, programID string) (string, bool, error)
	Cohort(ctx context.Context, id string) (Cohort, bool, error)
	ProgramOfCohort(ctx context.Context, cohortID string) (string, bool, error)
	IsEnrolled(ctx context.Context, rgm string) (bool, error)
	CohortOf(ctx context.Context, rgm string) (string, error)
	CohortsOf(ctx context.Context, rgms []string) (map[string]string, error)
}

type Accounts interface {
	StudentExists(ctx context.Context, rgm string) (bool, error)
	AdvisorExists(ctx context.Context, sub string) (bool, error)
	StudentNames(ctx context.Context, rgms []string) (map[string]string, error)
	AdvisorName(ctx context.Context, sub string) (string, bool, error)
}
