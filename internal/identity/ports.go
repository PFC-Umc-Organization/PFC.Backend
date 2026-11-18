package identity

import (
	"context"
	"errors"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
)

// ---------------------------------------------------------------------------
// Erros
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Diretório
// ---------------------------------------------------------------------------

// Resposta
type SignInResult struct {
	IDToken            string
	NewPasswordSession string
}

// Dependência
type Directory interface {
	SignIn(ctx context.Context, email, password string) (SignInResult, error)
	CompleteNewPassword(ctx context.Context, email, session, password string) (idToken string, err error)

	ForgotPassword(ctx context.Context, email string) error
	ConfirmForgotPassword(ctx context.Context, email, code, password string) error

	SignUp(ctx context.Context, name, email, password string) error
	ConfirmSignUp(ctx context.Context, email, code string) error
	ResendCode(ctx context.Context, email string) error

	CreateAccount(ctx context.Context, name, email string, role actor.Role) (sub string, err error)
	ListUsers(ctx context.Context) ([]User, error)
	SetName(ctx context.Context, username, name string) error
	SetRole(ctx context.Context, username string, role actor.Role) error

	SetUserMFA(ctx context.Context, username string, enabled bool) error

	Disable(ctx context.Context, username string) error
	Enable(ctx context.Context, username string) error

	ResendInvitation(ctx context.Context, email string) error
	UsersWithEmailPrefix(ctx context.Context, prefix string) ([]User, error)
	UsersWithSub(ctx context.Context, sub string) ([]User, error)
}

// ---------------------------------------------------------------------------
// Perfis
// ---------------------------------------------------------------------------

// Dependência
type Profiles interface {
	SaveStudent(ctx context.Context, rgm, name string) error
	SaveStaff(ctx context.Context, sub, name string, role actor.Role) error
	StudentNames(ctx context.Context, rgms []string) (map[string]string, error)
	Staff(ctx context.Context, sub string) (StaffProfile, bool, error)
}

// Modelo
type StaffProfile struct {
	Name string
	Role actor.Role
}

// ---------------------------------------------------------------------------
// Dependências
// ---------------------------------------------------------------------------

// Dependência
type Advising interface {
	MembersAdvisedBy(ctx context.Context, advisorSub string) ([]string, error)
}

// Dependência
type Academic interface {
	CohortOf(ctx context.Context, rgm string) (string, error)
	CohortsOf(ctx context.Context, rgms []string) (map[string]string, error)
}
