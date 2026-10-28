package identity

import (
	"context"
	"errors"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
)

// Directory errors classify Cognito's answers; the service picks the
// message for each operation.
var (
	errNotConfirmed     = errors.New("account not confirmed")
	errWeakPassword     = errors.New("password outside the policy")
	errEmailTaken       = errors.New("e-mail already registered")
	errSignUpRejected   = errors.New("pre sign-up rejected the account")
	errInvalidInput     = errors.New("invalid parameter")
	errThrottled        = errors.New("too many requests")
	errWrongCode        = errors.New("wrong confirmation code")
	errExpiredCode      = errors.New("expired confirmation code")
	errAlreadyConfirmed = errors.New("account already confirmed")
	errNoAccount        = errors.New("no such account")
	errSessionExpired   = errors.New("challenge session expired")
)

// SignInResult carries the ID token or, when Cognito asks for a new
// password, the session to answer it with.
type SignInResult struct {
	IDToken            string
	NewPasswordSession string
}

type Directory interface {
	SignIn(ctx context.Context, email, password string) (SignInResult, error)
	CompleteNewPassword(ctx context.Context, email, session, password string) (idToken string, err error)
	// ForgotPassword e-mails a reset code; ConfirmForgotPassword sets the new
	// password with it.
	ForgotPassword(ctx context.Context, email string) error
	ConfirmForgotPassword(ctx context.Context, email, code, password string) error
	// SignUp always creates a student; the Pre Sign-up Lambda checks the
	// RGM against the allowlist.
	SignUp(ctx context.Context, name, email, password string) error
	ConfirmSignUp(ctx context.Context, email, code string) error
	ResendCode(ctx context.Context, email string) error
	// CreateAccount sends Cognito's e-mail invitation with a temporary
	// password.
	CreateAccount(ctx context.Context, name, email string, role actor.Role) (sub string, err error)
	ListUsers(ctx context.Context) ([]User, error)
	SetName(ctx context.Context, username, name string) error
	SetRole(ctx context.Context, username string, role actor.Role) error
	// Disable also signs the account out of every device; tokens already
	// issued stay valid until they expire.
	Disable(ctx context.Context, username string) error
	Enable(ctx context.Context, username string) error
	// ResendInvitation e-mails a new temporary password to an account that
	// never signed in.
	ResendInvitation(ctx context.Context, email string) error
	UsersWithEmailPrefix(ctx context.Context, prefix string) ([]User, error)
	UsersWithSub(ctx context.Context, sub string) ([]User, error)
}

// Profiles keeps the names the pages show, so that showing a project costs
// no Cognito call: ListUsers allows a few calls per second for the whole
// pool. Written on sign-up, staff creation, rename and role change.
type Profiles interface {
	SaveStudent(ctx context.Context, rgm, name string) error
	SaveStaff(ctx context.Context, sub, name string, role actor.Role) error
	StudentNames(ctx context.Context, rgms []string) (map[string]string, error)
	Staff(ctx context.Context, sub string) (StaffProfile, bool, error)
}

type StaffProfile struct {
	Name string
	Role actor.Role
}

// Advising tells which students an advisor follows: the members of the
// projects they advise.
type Advising interface {
	MembersAdvisedBy(ctx context.Context, advisorSub string) ([]string, error)
}

type Academic interface {
	CohortOf(ctx context.Context, rgm string) (string, error)
	CohortsOf(ctx context.Context, rgms []string) (map[string]string, error)
}
