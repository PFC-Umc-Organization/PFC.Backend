package identity

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/parallel"
)

type Service struct {
	directory Directory
	academic  Academic
}

func NewService(directory Directory, academic Academic) *Service {
	return &Service{directory: directory, academic: academic}
}

// cohortsOf returns the cohort the student's RGM was pre-authorized in. A
// failed lookup must not block a login or the list: it is logged and the
// list stays empty.
func (s *Service) cohortsOf(ctx context.Context, rgm string) []string {
	cohort, err := s.academic.CohortOf(ctx, rgm)
	if err != nil {
		slog.WarnContext(ctx, "cohort lookup failed", "rgm", rgm, "error", err)
	}
	if cohort == "" {
		return []string{}
	}
	return []string{cohort}
}

func record(ctx context.Context, action, result, reason string, who *audit.Actor) {
	audit.Record(ctx, audit.Event{Action: action, Result: result, Reason: reason, Actor: who})
}

func (s *Service) Login(ctx context.Context, in Credentials) (LoginResult, error) {
	email := normalizeEmail(in.Email)
	if email == "" || in.Password == "" {
		return LoginResult{}, apperr.Invalid(msgCredentialsRequired)
	}
	result, err := s.directory.SignIn(ctx, email, in.Password)
	if errors.Is(err, errNotConfirmed) {
		// The frontend sends the student to the confirmation screen on 403.
		record(ctx, "auth.login", audit.Failure, "conta ainda não confirmada", &audit.Actor{Email: email})
		return LoginResult{}, apperr.Forbidden(msgNotConfirmed)
	}
	if err != nil {
		record(ctx, "auth.login", audit.Failure, "credenciais inválidas", &audit.Actor{Email: email})
		return LoginResult{}, apperr.Unauthorized(msgBadCredentials)
	}
	if result.NewPasswordSession != "" {
		record(ctx, "auth.login", audit.Failure, "troca da senha temporária pendente", &audit.Actor{Email: email})
		return LoginResult{Challenge: ChallengeNewPassword, Session: result.NewPasswordSession}, nil
	}
	session, err := s.session(ctx, result.IDToken)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{User: &session.User, Token: session.Token}, nil
}

func (s *Service) session(ctx context.Context, token string) (Session, error) {
	claims, err := claimsOf(token)
	if err != nil {
		return Session{}, apperr.Internal("Falha ao processar autenticação", err)
	}
	u := sessionUser(claims)
	if u.Role == actor.RoleStudent {
		u.CohortIDs = s.cohortsOf(ctx, u.RGM)
	}
	record(ctx, "auth.login", audit.Success, "", &audit.Actor{Sub: u.ID, Role: string(u.Role), Email: u.Email})
	return Session{User: u, Token: token}, nil
}

// CompleteNewPassword replaces the temporary password of an account created
// by an admin and signs the person in.
func (s *Service) CompleteNewPassword(ctx context.Context, in NewPasswordInput) (Session, error) {
	email := normalizeEmail(in.Email)
	if email == "" || in.Session == "" || in.Password == "" {
		return Session{}, apperr.Invalid(msgNewPasswordRequired)
	}
	token, err := s.directory.CompleteNewPassword(ctx, email, in.Session, in.Password)
	if err != nil {
		var appErr error
		switch {
		case errors.Is(err, errWeakPassword):
			appErr = apperr.Invalid(msgWeakPassword)
		case errors.Is(err, errSessionExpired):
			appErr = apperr.Unauthorized(msgSessionExpired)
		case errors.Is(err, errThrottled):
			appErr = apperr.TooManyRequests(msgThrottled)
		default:
			return Session{}, apperr.Internal("Falha ao definir a senha", err)
		}
		record(ctx, "auth.senha_definida", audit.Failure, message(appErr), &audit.Actor{Email: email})
		return Session{}, appErr
	}
	record(ctx, "auth.senha_definida", audit.Success, "", &audit.Actor{Email: email})
	return s.session(ctx, token)
}

// ForgotPassword answers the same whether or not the account exists, so the
// endpoint does not reveal who is registered.
func (s *Service) ForgotPassword(ctx context.Context, in ForgotPasswordInput) error {
	email := normalizeEmail(in.Email)
	if email == "" {
		return apperr.Invalid(msgEmailRequired)
	}
	err := s.directory.ForgotPassword(ctx, email)
	switch {
	case err == nil:
		record(ctx, "auth.senha_esquecida", audit.Success, "", &audit.Actor{Email: email})
	case errors.Is(err, errNoAccount), errors.Is(err, errInvalidInput):
		record(ctx, "auth.senha_esquecida", audit.Failure, "conta inexistente ou sem e-mail confirmado", &audit.Actor{Email: email})
	case errors.Is(err, errThrottled):
		return apperr.TooManyRequests(msgThrottled)
	default:
		return apperr.Internal("Falha ao enviar o código", err)
	}
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, in ResetPasswordInput) error {
	email := normalizeEmail(in.Email)
	code := strings.TrimSpace(in.Code)
	if email == "" || code == "" || in.Password == "" {
		return apperr.Invalid(msgResetRequired)
	}
	err := s.directory.ConfirmForgotPassword(ctx, email, code, in.Password)
	if err == nil {
		record(ctx, "auth.senha_redefinida", audit.Success, "", &audit.Actor{Email: email})
		return nil
	}
	var appErr error
	switch {
	case errors.Is(err, errWrongCode), errors.Is(err, errNoAccount):
		appErr = apperr.Invalid(msgWrongCode)
	case errors.Is(err, errExpiredCode):
		appErr = apperr.Invalid(msgExpiredCode)
	case errors.Is(err, errWeakPassword):
		appErr = apperr.Invalid(msgWeakPassword)
	case errors.Is(err, errThrottled):
		appErr = apperr.TooManyRequests(msgThrottled)
	default:
		return apperr.Internal("Falha ao redefinir a senha", err)
	}
	record(ctx, "auth.senha_redefinida", audit.Failure, message(appErr), &audit.Actor{Email: email})
	return appErr
}

// Me returns the signed-in user as the login does, with the cohort read now:
// a student pre-authorized after logging in sees it without logging in again.
func (s *Service) Me(ctx context.Context, who actor.Actor) (SessionUser, error) {
	if who.Sub == "" {
		return SessionUser{}, apperr.Unauthorized(msgNoSession)
	}
	u := SessionUser{
		ID: who.Sub, Name: who.Name, Email: who.Email, Role: roleOrStudent(string(who.Role)),
		Status: StatusActive, CohortIDs: []string{},
	}
	if u.Role == actor.RoleStudent {
		u.RGM = who.RGM()
		u.CohortIDs = s.cohortsOf(ctx, u.RGM)
	}
	return u, nil
}

func signUpError(err error) error {
	switch {
	case errors.Is(err, errWeakPassword):
		return apperr.Invalid(msgWeakPassword)
	case errors.Is(err, errEmailTaken):
		return apperr.Conflict("Já existe uma conta com esse e-mail. Use \"Entrar\" ou recupere a senha")
	case errors.Is(err, errSignUpRejected):
		return apperr.Forbidden("Cadastro não autorizado: use o e-mail <seu RGM>@alunos.umc.br e confirme com a coordenação se o seu RGM foi pré-autorizado")
	case errors.Is(err, errInvalidInput):
		return apperr.Invalid("Dados inválidos. Confira o nome e o e-mail")
	case errors.Is(err, errThrottled):
		return apperr.TooManyRequests(msgThrottled)
	}
	return apperr.Invalid("Não foi possível concluir o cadastro")
}

// confirmationError gives a wrong code, an unknown e-mail and an account
// already confirmed the same answer, so the endpoint does not reveal who is
// registered.
func confirmationError(err error) error {
	switch {
	case errors.Is(err, errWrongCode), errors.Is(err, errNoAccount), errors.Is(err, errAlreadyConfirmed):
		return apperr.Invalid(msgConfirmFailed)
	case errors.Is(err, errExpiredCode):
		return apperr.Invalid(msgExpiredCode)
	case errors.Is(err, errThrottled):
		return apperr.TooManyRequests(msgThrottled)
	}
	return apperr.Invalid("Não foi possível confirmar a conta")
}

func message(err error) string {
	e, _ := apperr.As(err)
	return e.Message
}

func (s *Service) SignUp(ctx context.Context, in SignUpInput) error {
	name := strings.TrimSpace(in.Name)
	email := normalizeEmail(in.Email)
	if name == "" || email == "" || in.Password == "" {
		return apperr.Invalid(msgSignUpRequired)
	}
	if err := s.directory.SignUp(ctx, name, email, in.Password); err != nil {
		slog.WarnContext(ctx, "sign-up failed", "email", email, "error", err)
		appErr := signUpError(err)
		record(ctx, "auth.cadastro", audit.Failure, message(appErr), &audit.Actor{Email: email})
		return appErr
	}
	record(ctx, "auth.cadastro", audit.Success, "", &audit.Actor{Email: email})
	return nil
}

func (s *Service) Confirm(ctx context.Context, in Confirmation) error {
	email := normalizeEmail(in.Email)
	code := strings.TrimSpace(in.Code)
	if email == "" || code == "" {
		return apperr.Invalid(msgConfirmRequired)
	}
	if err := s.directory.ConfirmSignUp(ctx, email, code); err != nil {
		slog.WarnContext(ctx, "confirmation failed", "email", email, "error", err)
		appErr := confirmationError(err)
		record(ctx, "auth.conta_confirmada", audit.Failure, message(appErr), &audit.Actor{Email: email})
		return appErr
	}
	record(ctx, "auth.conta_confirmada", audit.Success, "", &audit.Actor{Email: email})
	return nil
}

func (s *Service) ResendCode(ctx context.Context, in ResendInput) error {
	email := normalizeEmail(in.Email)
	if email == "" {
		return apperr.Invalid(msgEmailRequired)
	}
	err := s.directory.ResendCode(ctx, email)
	switch {
	case err == nil:
		record(ctx, "auth.codigo_reenviado", audit.Success, "", &audit.Actor{Email: email})
	case errors.Is(err, errNoAccount), errors.Is(err, errInvalidInput):
		// Cognito answers InvalidParameter for an account already confirmed.
		record(ctx, "auth.codigo_reenviado", audit.Failure, "conta inexistente ou já confirmada", &audit.Actor{Email: email})
	case errors.Is(err, errThrottled):
		return apperr.TooManyRequests(msgThrottled)
	default:
		return apperr.Internal("Falha ao reenviar o código", err)
	}
	return nil
}

// ListUsers is for the academic team; students get their group's names from
// the project itself.
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	users, err := s.directory.ListUsers(ctx)
	if err != nil {
		return nil, apperr.Internal("Falha ao listar usuários", err)
	}
	rgms := make([]string, 0, len(users))
	for _, u := range users {
		if u.RGM != "" {
			rgms = append(rgms, u.RGM)
		}
	}
	// Like cohortsOf, a failed lookup leaves the cohorts empty.
	cohorts, err := s.academic.CohortsOf(ctx, rgms)
	if err != nil {
		slog.WarnContext(ctx, "cohort lookup failed", "error", err)
	}
	for i := range users {
		users[i].CohortIDs = []string{}
		if c := cohorts[users[i].RGM]; c != "" {
			users[i].CohortIDs = []string{c}
		}
	}
	slices.SortStableFunc(users, func(a, b User) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return users, nil
}

func (s *Service) CreateAccount(ctx context.Context, in NewAccount) error {
	name := strings.TrimSpace(in.Name)
	email := normalizeEmail(in.Email)
	switch {
	case name == "" || email == "":
		return apperr.Invalid(msgAccountRequired)
	case !strings.HasSuffix(email, "@umc.br"):
		// @alunos.umc.br is the students' domain.
		return apperr.Invalid(msgInstitutionalEmail)
	case in.Role != actor.RoleAdvisor && in.Role != actor.RoleAdmin:
		return apperr.Invalid(msgStaffRole)
	}
	details := map[string]any{"email": email, "perfil": string(in.Role)}
	if err := s.directory.CreateAccount(ctx, name, email, in.Role); err != nil {
		var appErr error
		switch {
		case errors.Is(err, errEmailTaken):
			appErr = apperr.Conflict("Já existe uma conta com esse e-mail")
		case errors.Is(err, errInvalidInput):
			appErr = apperr.Invalid("E-mail inválido")
		default:
			appErr = apperr.Internal("Falha ao criar a conta", err)
		}
		slog.WarnContext(ctx, "account creation failed", "email", email, "error", err)
		audit.Record(ctx, audit.Event{Action: "usuario.criado", Result: audit.Failure, Reason: message(appErr), Details: details})
		return appErr
	}
	audit.Record(ctx, audit.Event{Action: "usuario.criado", Result: audit.Success, Details: details})
	return nil
}

// StudentExists accepts as group member an RGM that owns a student account
// even outside the allowlist (created before it, or removed later).
func (s *Service) StudentExists(ctx context.Context, rgm string) (bool, error) {
	if !digitsOnly.MatchString(rgm) {
		return false, nil
	}
	users, err := s.directory.UsersWithEmailPrefix(ctx, rgm+"@")
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(users, func(u User) bool { return u.Role == actor.RoleStudent && u.RGM == rgm }), nil
}

// StudentNames maps each RGM with a student account to its name.
func (s *Service) StudentNames(ctx context.Context, rgms []string) (map[string]string, error) {
	found, err := parallel.Map(rgms, func(rgm string) (User, error) {
		users, err := s.directory.UsersWithEmailPrefix(ctx, rgm+"@")
		if err != nil {
			return User{}, err
		}
		i := slices.IndexFunc(users, func(u User) bool { return u.Role == actor.RoleStudent && u.RGM == rgm })
		if i < 0 {
			return User{}, nil
		}
		return users[i], nil
	})
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(found))
	for _, u := range found {
		if u.RGM != "" {
			names[u.RGM] = u.Name
		}
	}
	return names, nil
}

func (s *Service) AdvisorName(ctx context.Context, sub string) (string, bool, error) {
	if !subFormat.MatchString(sub) {
		return "", false, nil
	}
	users, err := s.directory.UsersWithSub(ctx, sub)
	if err != nil {
		return "", false, err
	}
	i := slices.IndexFunc(users, func(u User) bool { return u.ID == sub && u.Role == actor.RoleAdvisor })
	if i < 0 {
		return "", false, nil
	}
	return users[i].Name, true, nil
}

func (s *Service) AdvisorExists(ctx context.Context, sub string) (bool, error) {
	if !subFormat.MatchString(sub) {
		return false, nil
	}
	users, err := s.directory.UsersWithSub(ctx, sub)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(users, func(u User) bool { return u.ID == sub && u.Role == actor.RoleAdvisor }), nil
}
