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
	CreateAccount(ctx context.Context, name, email string, role actor.Role) error
	ListUsers(ctx context.Context) ([]User, error)
	UsersWithEmailPrefix(ctx context.Context, prefix string) ([]User, error)
	UsersWithSub(ctx context.Context, sub string) ([]User, error)
}

type Academic interface {
	CohortOf(ctx context.Context, rgm string) (string, error)
	CohortsOf(ctx context.Context, rgms []string) (map[string]string, error)
}
