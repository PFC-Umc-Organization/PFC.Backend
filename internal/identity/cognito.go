package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	cip "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
)

// ListUsers returns at most 60 accounts per page.
const pageSize = 60

// Cognito is the Directory. ListUsers and AdminCreateUser are IAM-authorized
// admin calls, which is why they need the user pool id.
type Cognito struct {
	client     *cip.Client
	clientID   string
	userPoolID string
}

func NewCognito(client *cip.Client, clientID, userPoolID string) *Cognito {
	return &Cognito{client: client, clientID: clientID, userPoolID: userPoolID}
}

func classify(err error, target error) error { return fmt.Errorf("%w: %v", target, err) }

func (c *Cognito) SignIn(ctx context.Context, email, password string) (SignInResult, error) {
	out, err := c.client.InitiateAuth(ctx, &cip.InitiateAuthInput{
		AuthFlow:       types.AuthFlowTypeUserPasswordAuth,
		ClientId:       aws.String(c.clientID),
		AuthParameters: map[string]string{"USERNAME": email, "PASSWORD": password},
	})
	var notConfirmed *types.UserNotConfirmedException
	if errors.As(err, &notConfirmed) {
		return SignInResult{}, classify(err, errNotConfirmed)
	}
	if err != nil {
		return SignInResult{}, err
	}
	switch out.ChallengeName {
	case "":
	case types.ChallengeNameTypeNewPasswordRequired:
		return SignInResult{NewPasswordSession: aws.ToString(out.Session)}, nil
	default:
		return SignInResult{}, fmt.Errorf("unsupported challenge %s", out.ChallengeName)
	}
	return tokenOf(out.AuthenticationResult)
}

func tokenOf(r *types.AuthenticationResultType) (SignInResult, error) {
	if r == nil || r.IdToken == nil {
		return SignInResult{}, errors.New("Cognito returned no token")
	}
	return SignInResult{IDToken: *r.IdToken}, nil
}

func (c *Cognito) CompleteNewPassword(ctx context.Context, email, session, password string) (string, error) {
	out, err := c.client.RespondToAuthChallenge(ctx, &cip.RespondToAuthChallengeInput{
		ClientId:           aws.String(c.clientID),
		ChallengeName:      types.ChallengeNameTypeNewPasswordRequired,
		Session:            aws.String(session),
		ChallengeResponses: map[string]string{"USERNAME": email, "NEW_PASSWORD": password},
	})
	var (
		weak      *types.InvalidPasswordException
		expired   *types.NotAuthorizedException
		throttled *types.TooManyRequestsException
	)
	switch {
	case errors.As(err, &weak):
		return "", classify(err, errWeakPassword)
	case errors.As(err, &expired):
		return "", classify(err, errSessionExpired)
	case errors.As(err, &throttled):
		return "", classify(err, errThrottled)
	case err != nil:
		return "", err
	}
	r, err := tokenOf(out.AuthenticationResult)
	return r.IDToken, err
}

func (c *Cognito) ForgotPassword(ctx context.Context, email string) error {
	_, err := c.client.ForgotPassword(ctx, &cip.ForgotPasswordInput{ClientId: aws.String(c.clientID), Username: aws.String(email)})
	return classifyConfirmation(err)
}

func (c *Cognito) ConfirmForgotPassword(ctx context.Context, email, code, password string) error {
	_, err := c.client.ConfirmForgotPassword(ctx, &cip.ConfirmForgotPasswordInput{
		ClientId:         aws.String(c.clientID),
		Username:         aws.String(email),
		ConfirmationCode: aws.String(code),
		Password:         aws.String(password),
	})
	var weak *types.InvalidPasswordException
	if errors.As(err, &weak) {
		return classify(err, errWeakPassword)
	}
	return classifyConfirmation(err)
}

func signUpAttributes(name, email string) []types.AttributeType {
	return []types.AttributeType{
		{Name: aws.String("email"), Value: aws.String(email)},
		{Name: aws.String("name"), Value: aws.String(name)},
		{Name: aws.String("custom:perfil"), Value: aws.String(string(actor.RoleStudent))},
	}
}

func (c *Cognito) SignUp(ctx context.Context, name, email, password string) error {
	_, err := c.client.SignUp(ctx, &cip.SignUpInput{
		ClientId:       aws.String(c.clientID),
		Username:       aws.String(email),
		Password:       aws.String(password),
		UserAttributes: signUpAttributes(name, email),
	})
	return classifySignUp(err)
}

func classifySignUp(err error) error {
	var (
		weak      *types.InvalidPasswordException
		taken     *types.UsernameExistsException
		rejected  *types.UserLambdaValidationException
		invalid   *types.InvalidParameterException
		throttled *types.TooManyRequestsException
	)
	switch {
	case err == nil:
		return nil
	case errors.As(err, &weak):
		return classify(err, errWeakPassword)
	case errors.As(err, &taken):
		return classify(err, errEmailTaken)
	case errors.As(err, &rejected):
		return classify(err, errSignUpRejected)
	case errors.As(err, &invalid):
		return classify(err, errInvalidInput)
	case errors.As(err, &throttled):
		return classify(err, errThrottled)
	}
	return err
}

func (c *Cognito) ConfirmSignUp(ctx context.Context, email, code string) error {
	_, err := c.client.ConfirmSignUp(ctx, &cip.ConfirmSignUpInput{
		ClientId:         aws.String(c.clientID),
		Username:         aws.String(email),
		ConfirmationCode: aws.String(code),
	})
	return classifyConfirmation(err)
}

func (c *Cognito) ResendCode(ctx context.Context, email string) error {
	_, err := c.client.ResendConfirmationCode(ctx, &cip.ResendConfirmationCodeInput{
		ClientId: aws.String(c.clientID),
		Username: aws.String(email),
	})
	return classifyConfirmation(err)
}

func classifyConfirmation(err error) error {
	var (
		wrong     *types.CodeMismatchException
		expired   *types.ExpiredCodeException
		confirmed *types.NotAuthorizedException
		missing   *types.UserNotFoundException
		limit     *types.LimitExceededException
		throttled *types.TooManyRequestsException
		attempts  *types.TooManyFailedAttemptsException
		invalid   *types.InvalidParameterException
	)
	switch {
	case err == nil:
		return nil
	case errors.As(err, &wrong):
		return classify(err, errWrongCode)
	case errors.As(err, &expired):
		return classify(err, errExpiredCode)
	case errors.As(err, &confirmed):
		// ConfirmSignUp on a CONFIRMED account answers NotAuthorized.
		return classify(err, errAlreadyConfirmed)
	case errors.As(err, &missing):
		return classify(err, errNoAccount)
	case errors.As(err, &limit), errors.As(err, &throttled), errors.As(err, &attempts):
		return classify(err, errThrottled)
	case errors.As(err, &invalid):
		return classify(err, errInvalidInput)
	}
	return err
}

func (c *Cognito) CreateAccount(ctx context.Context, name, email string, role actor.Role) error {
	_, err := c.client.AdminCreateUser(ctx, &cip.AdminCreateUserInput{
		UserPoolId: aws.String(c.userPoolID),
		Username:   aws.String(email),
		UserAttributes: []types.AttributeType{
			{Name: aws.String("email"), Value: aws.String(email)},
			{Name: aws.String("email_verified"), Value: aws.String("true")},
			{Name: aws.String("name"), Value: aws.String(name)},
			{Name: aws.String("custom:perfil"), Value: aws.String(string(role))},
		},
	})
	var (
		taken   *types.UsernameExistsException
		invalid *types.InvalidParameterException
	)
	switch {
	case errors.As(err, &taken):
		return classify(err, errEmailTaken)
	case errors.As(err, &invalid):
		return classify(err, errInvalidInput)
	}
	return err
}

func toUser(u types.UserType) User {
	attrs := make(map[string]string, len(u.Attributes))
	for _, a := range u.Attributes {
		attrs[aws.ToString(a.Name)] = aws.ToString(a.Value)
	}
	user := userFromAttributes(attrs, aws.ToString(u.Username), u.Enabled, u.UserStatus == types.UserStatusTypeConfirmed)
	user.FirstAccessPending = u.UserStatus == types.UserStatusTypeForceChangePassword
	return user
}

func classifyAdmin(err error) error {
	var (
		missing   *types.UserNotFoundException
		invalid   *types.InvalidParameterException
		throttled *types.TooManyRequestsException
	)
	switch {
	case err == nil:
		return nil
	case errors.As(err, &missing):
		return classify(err, errNoAccount)
	case errors.As(err, &invalid):
		return classify(err, errInvalidInput)
	case errors.As(err, &throttled):
		return classify(err, errThrottled)
	}
	return err
}

func (c *Cognito) setAttribute(ctx context.Context, username, name, value string) error {
	_, err := c.client.AdminUpdateUserAttributes(ctx, &cip.AdminUpdateUserAttributesInput{
		UserPoolId:     aws.String(c.userPoolID),
		Username:       aws.String(username),
		UserAttributes: []types.AttributeType{{Name: aws.String(name), Value: aws.String(value)}},
	})
	return classifyAdmin(err)
}

func (c *Cognito) SetName(ctx context.Context, username, name string) error {
	return c.setAttribute(ctx, username, "name", name)
}

func (c *Cognito) SetRole(ctx context.Context, username string, role actor.Role) error {
	return c.setAttribute(ctx, username, "custom:perfil", string(role))
}

func (c *Cognito) Disable(ctx context.Context, username string) error {
	if _, err := c.client.AdminDisableUser(ctx, &cip.AdminDisableUserInput{
		UserPoolId: aws.String(c.userPoolID), Username: aws.String(username),
	}); err != nil {
		return classifyAdmin(err)
	}
	_, err := c.client.AdminUserGlobalSignOut(ctx, &cip.AdminUserGlobalSignOutInput{
		UserPoolId: aws.String(c.userPoolID), Username: aws.String(username),
	})
	return classifyAdmin(err)
}

func (c *Cognito) Enable(ctx context.Context, username string) error {
	_, err := c.client.AdminEnableUser(ctx, &cip.AdminEnableUserInput{
		UserPoolId: aws.String(c.userPoolID), Username: aws.String(username),
	})
	return classifyAdmin(err)
}

func (c *Cognito) ResendInvitation(ctx context.Context, email string) error {
	_, err := c.client.AdminCreateUser(ctx, &cip.AdminCreateUserInput{
		UserPoolId:    aws.String(c.userPoolID),
		Username:      aws.String(email),
		MessageAction: types.MessageActionTypeResend,
	})
	return classifyAdmin(err)
}

func (c *Cognito) listUsers(ctx context.Context, filter string, limit int32) ([]User, error) {
	users := []User{}
	var token *string
	for {
		// No AttributesToGet on purpose: Cognito rejects "name" there
		// (InvalidParameterException) even with it in the schema.
		in := &cip.ListUsersInput{UserPoolId: aws.String(c.userPoolID), Limit: aws.Int32(limit), PaginationToken: token}
		if filter != "" {
			in.Filter = aws.String(filter)
		}
		out, err := c.client.ListUsers(ctx, in)
		if err != nil {
			return nil, err
		}
		for _, u := range out.Users {
			users = append(users, toUser(u))
		}
		if filter != "" || out.PaginationToken == nil || *out.PaginationToken == "" {
			return users, nil
		}
		token = out.PaginationToken
	}
}

func (c *Cognito) ListUsers(ctx context.Context) ([]User, error) {
	return c.listUsers(ctx, "", pageSize)
}

// UsersWithEmailPrefix uses ^= ("starts with") to match <rgm>@ on any
// domain.
func (c *Cognito) UsersWithEmailPrefix(ctx context.Context, prefix string) ([]User, error) {
	return c.listUsers(ctx, fmt.Sprintf(`email ^= "%s"`, prefix), pageSize)
}

func (c *Cognito) UsersWithSub(ctx context.Context, sub string) ([]User, error) {
	return c.listUsers(ctx, fmt.Sprintf(`sub = "%s"`, sub), 1)
}
