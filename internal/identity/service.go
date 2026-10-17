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

func (s *Service) Login(ctx context.Context, in Credentials) (Session, error) {
	email := normalizeEmail(in.Email)
	if email == "" || in.Password == "" {
		return Session{}, apperr.Invalid(msgCredentialsRequired)
	}
	token, err := s.directory.SignIn(ctx, email, in.Password)
	if errors.Is(err, errNotConfirmed) {
		// The frontend sends the student to the confirmation screen on 403.
		record(ctx, "auth.login", audit.Failure, "conta ainda não confirmada", &audit.Actor{Email: email})
		return Session{}, apperr.Forbidden(msgNotConfirmed)
	}
	if err != nil {
		record(ctx, "auth.login", audit.Failure, "credenciais inválidas", &audit.Actor{Email: email})
		return Session{}, apperr.Unauthorized(msgBadCredentials)
	}
	claims, err := claimsOf(token)
	if err != nil {
		return Session{}, apperr.Internal("falha ao processar autenticação", err)
	}
	u := sessionUser(claims)
	if u.Role == actor.RoleStudent {
		u.CohortIDs = s.cohortsOf(ctx, u.RGM)
	}
	record(ctx, "auth.login", audit.Success, "", &audit.Actor{Sub: u.ID, Role: string(u.Role), Email: u.Email})
	return Session{User: u, Token: token}, nil
}

func signUpError(err error) error {
	switch {
	case errors.Is(err, errWeakPassword):
		return apperr.Invalid("a senha precisa ter ao menos 8 caracteres, com letra maiúscula, letra minúscula, número e símbolo")
	case errors.Is(err, errEmailTaken):
		return apperr.Conflict("já existe uma conta com esse e-mail — use \"Entrar\" ou recupere a senha")
	case errors.Is(err, errSignUpRejected):
		return apperr.Forbidden("cadastro não autorizado: use o e-mail <seu RGM>@alunos.umc.br e confirme com a coordenação se o seu RGM foi pré-autorizado")
	case errors.Is(err, errInvalidInput):
		return apperr.Invalid("dados inválidos — confira o nome e o e-mail")
	case errors.Is(err, errThrottled):
		return apperr.TooManyRequests(msgThrottled)
	}
	return apperr.Invalid("não foi possível concluir o cadastro")
}

func confirmationError(err error) error {
	switch {
	case errors.Is(err, errWrongCode):
		return apperr.Invalid("código incorreto — confira o e-mail e tente de novo")
	case errors.Is(err, errExpiredCode):
		return apperr.Invalid("código expirado — peça um código novo")
	case errors.Is(err, errAlreadyConfirmed):
		return apperr.Conflict("esta conta já está confirmada — é só entrar")
	case errors.Is(err, errNoAccount):
		return apperr.NotFound("não há cadastro com esse e-mail")
	case errors.Is(err, errThrottled):
		return apperr.TooManyRequests(msgThrottled)
	}
	return apperr.Invalid("não foi possível confirmar a conta")
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
	if err := s.directory.ResendCode(ctx, email); err != nil {
		slog.WarnContext(ctx, "code resend failed", "email", email, "error", err)
		return confirmationError(err)
	}
	return nil
}

// ListUsers is open to every signed-in user: students need the names of
// their group and advisor. E-mails are personal data, shown to staff only.
func (s *Service) ListUsers(ctx context.Context, viewer actor.Actor) ([]User, error) {
	users, err := s.directory.ListUsers(ctx)
	if err != nil {
		return nil, apperr.Internal("falha ao listar usuários", err)
	}
	staff := viewer.HasRole(actor.RoleAdmin, actor.RoleAdvisor)
	for i := range users {
		if users[i].RGM != "" {
			users[i].CohortIDs = s.cohortsOf(ctx, users[i].RGM)
		}
		if !staff {
			users[i].Email = ""
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
			appErr = apperr.Conflict("já existe uma conta com esse e-mail")
		case errors.Is(err, errInvalidInput):
			appErr = apperr.Invalid("e-mail inválido")
		default:
			appErr = apperr.Internal("falha ao criar a conta", err)
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
