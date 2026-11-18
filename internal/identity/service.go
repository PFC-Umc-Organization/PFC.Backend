package identity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

// ---------------------------------------------------------------------------
// Serviço
// ---------------------------------------------------------------------------

// Serviço
type Service struct {
	directory Directory
	academic  Academic
	advising  Advising
	profiles  Profiles
}

// Construção
func NewService(directory Directory, academic Academic, profiles Profiles) *Service {
	return &Service{directory: directory, academic: academic, profiles: profiles}
}

// Configuração
func (s *Service) UseAdvising(a Advising) { s.advising = a }

// Consulta
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

// Auditoria
func record(ctx context.Context, action, result, reason string, who *audit.Actor) {
	audit.Record(ctx, audit.Event{Action: action, Result: result, Reason: reason, Actor: who})
}

// ---------------------------------------------------------------------------
// Autenticação
// ---------------------------------------------------------------------------

// Login
func (s *Service) Login(ctx context.Context, in Credentials) (LoginResult, error) {
	email := normalizeEmail(in.Email)
	if email == "" || in.Password == "" {
		return LoginResult{}, apperr.Invalid(msgCredentialsRequired)
	}
	result, err := s.directory.SignIn(ctx, email, in.Password)
	if errors.Is(err, errNotConfirmed) {
		record(ctx, "auth.login", audit.Failure, "conta ainda não confirmada", &audit.Actor{Email: email})
		return LoginResult{}, apperr.Forbidden(msgNotConfirmed)
	}
	if err != nil {

		slog.WarnContext(ctx, "sign-in failed", "email", email, "error", err)
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

// Montagem
func (s *Service) session(ctx context.Context, token string) (Session, error) {
	claims, err := claimsOf(token)
	if err != nil {
		return Session{}, apperr.Internal("Não foi possível processar a autenticação. Tente novamente em instantes.", err)
	}
	u := sessionUser(claims)
	if u.Role == actor.RoleStudent {
		u.CohortIDs = s.cohortsOf(ctx, u.RGM)
	}
	record(ctx, "auth.login", audit.Success, "", &audit.Actor{Sub: u.ID, Role: string(u.Role), Email: u.Email})
	return Session{User: u, Token: token}, nil
}

// Nova senha
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
			return Session{}, apperr.Internal("Não foi possível definir a senha. Tente novamente em instantes.", err)
		}
		record(ctx, "auth.senha_definida", audit.Failure, message(appErr), &audit.Actor{Email: email})
		return Session{}, appErr
	}
	record(ctx, "auth.senha_definida", audit.Success, "", &audit.Actor{Email: email})
	return s.session(ctx, token)
}

// ---------------------------------------------------------------------------
// Senhas
// ---------------------------------------------------------------------------

// Recuperação
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
		return apperr.Internal("Não foi possível enviar o código. Tente novamente em instantes.", err)
	}
	return nil
}

// Redefinição
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
		return apperr.Internal("Não foi possível redefinir a senha. Tente novamente em instantes.", err)
	}
	record(ctx, "auth.senha_redefinida", audit.Failure, message(appErr), &audit.Actor{Email: email})
	return appErr
}

// ---------------------------------------------------------------------------
// Sessão
// ---------------------------------------------------------------------------

// Perfil
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

// ---------------------------------------------------------------------------
// Cadastro
// ---------------------------------------------------------------------------

// Tradução
func signUpError(err error) error {
	switch {
	case errors.Is(err, errWeakPassword):
		return apperr.Invalid(msgWeakPassword)
	case errors.Is(err, errEmailTaken):
		return apperr.Conflict("Já existe uma conta com este e-mail. Entre com a sua senha ou use \"Esqueci minha senha\".")
	case errors.Is(err, errSignUpRejected):
		return apperr.Forbidden("Cadastro não autorizado. Use o e-mail no formato RGM@alunos.umc.br e confirme com a coordenação se o seu RGM foi pré-autorizado.")
	case errors.Is(err, errInvalidInput):
		return apperr.Invalid("Dados inválidos. Confira o nome e o e-mail.")
	case errors.Is(err, errThrottled):
		return apperr.TooManyRequests(msgThrottled)
	}
	return apperr.Invalid("Não foi possível concluir o cadastro. Tente novamente em instantes.")
}

// Tradução
func confirmationError(err error) error {
	switch {
	case errors.Is(err, errWrongCode), errors.Is(err, errNoAccount), errors.Is(err, errAlreadyConfirmed):
		return apperr.Invalid(msgConfirmFailed)
	case errors.Is(err, errExpiredCode):
		return apperr.Invalid(msgExpiredCode)
	case errors.Is(err, errThrottled):
		return apperr.TooManyRequests(msgThrottled)
	}
	return apperr.Invalid("Não foi possível confirmar a conta. Tente novamente em instantes.")
}

// Mensagem
func message(err error) string {
	e, _ := apperr.As(err)
	return e.Message
}

// Cadastro
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

	if err := s.profiles.SaveStudent(ctx, actor.RGMFromEmail(email), actor.DisplayName(name, email)); err != nil {
		slog.ErrorContext(ctx, "student name not saved", "email", email, "error", err)
	}
	record(ctx, "auth.cadastro", audit.Success, "", &audit.Actor{Email: email})
	return nil
}

// Confirmação
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

// Reenvio
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

		record(ctx, "auth.codigo_reenviado", audit.Failure, "conta inexistente ou já confirmada", &audit.Actor{Email: email})
	case errors.Is(err, errThrottled):
		return apperr.TooManyRequests(msgThrottled)
	default:
		return apperr.Internal("Não foi possível reenviar o código. Tente novamente em instantes.", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Contas
// ---------------------------------------------------------------------------

// Listagem
func (s *Service) ListUsers(ctx context.Context, viewer actor.Actor, includeMFA bool) ([]User, error) {
	users, err := s.directory.ListUsers(ctx)
	if err != nil {
		return nil, apperr.Internal("Não foi possível carregar os usuários. Tente novamente em instantes.", err)
	}
	if !viewer.HasRole(actor.RoleAdmin) {
		if users, err = s.advisees(ctx, viewer, users); err != nil {
			return nil, err
		}
	}
	rgms := make([]string, 0, len(users))
	for _, u := range users {
		if u.RGM != "" {
			rgms = append(rgms, u.RGM)
		}
	}

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
	if includeMFA && viewer.HasRole(actor.RoleAdmin, actor.RoleAdvisor) {
		s.fillMFA(ctx, users)
	}
	slices.SortStableFunc(users, func(a, b User) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return users, nil
}

// Leitura
func (s *Service) fillMFA(ctx context.Context, users []User) {
	mfa, err := s.profiles.MFAEnabled(ctx, users)
	if err != nil {
		slog.WarnContext(ctx, "mfa flags not loaded", "error", fmt.Errorf("mfa flags: %w", err))
		return
	}
	for i := range users {
		users[i].MFAEnabled = mfa[users[i].ID]
	}
}

// Consulta
func (s *Service) advisees(ctx context.Context, advisor actor.Actor, users []User) ([]User, error) {
	if s.advising == nil || !advisor.HasRole(actor.RoleAdvisor) {
		return []User{}, nil
	}
	members, err := s.advising.MembersAdvisedBy(ctx, advisor.Sub)
	if err != nil {
		return nil, apperr.Internal("Não foi possível carregar os usuários. Tente novamente em instantes.", err)
	}
	return slices.DeleteFunc(users, func(u User) bool {
		return u.Role != actor.RoleStudent || !slices.Contains(members, u.RGM)
	}), nil
}

// Auditoria
func accountResource(id string) *audit.Resource { return &audit.Resource{Type: "usuario", ID: id} }

// Consulta
func (s *Service) account(ctx context.Context, id string) (User, error) {
	if !subFormat.MatchString(id) {
		return User{}, apperr.NotFound(msgAccountNotFound)
	}
	users, err := s.directory.UsersWithSub(ctx, id)
	if err != nil {
		return User{}, apperr.Internal("Não foi possível carregar a conta. Tente novamente em instantes.", err)
	}
	i := slices.IndexFunc(users, func(u User) bool { return u.ID == id })
	if i < 0 {
		return User{}, apperr.NotFound(msgAccountNotFound)
	}
	u := users[i]
	if u.Username == "" {
		u.Username = u.ID
	}
	return u, nil
}

// Verificação
func (s *Service) lastActiveAdmin(ctx context.Context, u User) (bool, error) {
	if u.Role != actor.RoleAdmin || u.Status != StatusActive {
		return false, nil
	}
	users, err := s.directory.ListUsers(ctx)
	if err != nil {
		return false, err
	}
	for _, other := range users {
		if other.ID != u.ID && other.Role == actor.RoleAdmin && other.Status == StatusActive && !other.FirstAccessPending {
			return false, nil
		}
	}
	return true, nil
}

// Gravação
func (s *Service) saveName(ctx context.Context, u User) error {
	if u.Role == actor.RoleStudent {
		return s.profiles.SaveStudent(ctx, u.RGM, u.Name)
	}
	return s.profiles.SaveStaff(ctx, u.ID, u.Name, u.Role)
}

// Tradução
func adminError(err error) error {
	switch {
	case errors.Is(err, errNoAccount):
		return apperr.NotFound(msgAccountNotFound)
	case errors.Is(err, errThrottled):
		return apperr.TooManyRequests(msgThrottled)
	}
	return apperr.Internal("Não foi possível alterar a conta. Tente novamente em instantes.", err)
}

// Recusa
func refuse(ctx context.Context, action, id string, err error) error {
	audit.Record(ctx, audit.Event{Action: action, Result: audit.Failure, Reason: message(err), Resource: accountResource(id)})
	return err
}

// Renomeação
func (s *Service) RenameAccount(ctx context.Context, id string, in NameInput) error {
	const action = "usuario.nome_alterado"
	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		return apperr.Invalid(msgNameRequired)
	case utf8.RuneCountInString(name) > maxAccountNameLen:
		return apperr.Invalid(msgNameTooLong)
	}
	u, err := s.account(ctx, id)
	if err != nil {
		return err
	}
	if err := s.directory.SetName(ctx, u.Username, name); err != nil {
		return refuse(ctx, action, id, adminError(err))
	}
	u.Name = name
	if err := s.saveName(ctx, u); err != nil {
		return apperr.Internal(msgNameNotSaved, err)
	}
	audit.Record(ctx, audit.Event{Action: action, Result: audit.Success, Resource: accountResource(id)})
	return nil
}

// Troca
func (s *Service) ChangeRole(ctx context.Context, admin actor.Actor, id string, in RoleInput) error {
	const action = "usuario.perfil_alterado"
	if in.Role != actor.RoleAdvisor && in.Role != actor.RoleAdmin {
		return apperr.Invalid(msgStaffRole)
	}
	if id == admin.Sub {
		return refuse(ctx, action, id, apperr.Conflict(msgOwnRole))
	}
	u, err := s.account(ctx, id)
	if err != nil {
		return err
	}
	switch {
	case u.Role == actor.RoleStudent:
		return refuse(ctx, action, id, apperr.Conflict(msgStudentRole))
	case u.Role == in.Role:
		return apperr.Conflict(msgSameRole)
	}
	last, err := s.lastActiveAdmin(ctx, u)
	if err != nil {
		return apperr.Internal("Não foi possível alterar a conta. Tente novamente em instantes.", err)
	}
	if last {
		return refuse(ctx, action, id, apperr.Conflict(msgLastAdmin))
	}
	if err := s.directory.SetRole(ctx, u.Username, in.Role); err != nil {
		return refuse(ctx, action, id, adminError(err))
	}
	from := u.Role
	u.Role = in.Role
	if err := s.saveName(ctx, u); err != nil {
		return apperr.Internal(msgNameNotSaved, err)
	}
	audit.Record(ctx, audit.Event{Action: action, Result: audit.Success, Resource: accountResource(id),
		Details: map[string]any{"de": string(from), "para": string(in.Role)}})
	return nil
}

// Desativação
func (s *Service) DisableAccount(ctx context.Context, admin actor.Actor, id string) error {
	const action = "usuario.desativado"
	if id == admin.Sub {
		return refuse(ctx, action, id, apperr.Conflict(msgOwnDisable))
	}
	u, err := s.account(ctx, id)
	if err != nil {
		return err
	}
	if u.Status != StatusActive {
		return apperr.Conflict(msgAlreadyDisabled)
	}
	last, err := s.lastActiveAdmin(ctx, u)
	if err != nil {
		return apperr.Internal("Não foi possível alterar a conta. Tente novamente em instantes.", err)
	}
	if last {
		return refuse(ctx, action, id, apperr.Conflict(msgLastAdmin))
	}
	if err := s.directory.Disable(ctx, u.Username); err != nil {
		return refuse(ctx, action, id, adminError(err))
	}
	audit.Record(ctx, audit.Event{Action: action, Result: audit.Success, Resource: accountResource(id)})
	return nil
}

// Alternância
func (s *Service) SetMFA(ctx context.Context, id string, in MFAInput) (bool, error) {
	const action = "usuario.mfa_alterado"
	if in.Enabled == nil {
		return false, apperr.Invalid(msgMFAEnabledRequired)
	}
	enabled := *in.Enabled
	u, err := s.account(ctx, id)
	if err != nil {
		return false, err
	}
	if err := s.directory.SetUserMFA(ctx, u.Username, enabled); err != nil {
		if enabled && errors.Is(err, errInvalidInput) {
			return false, refuse(ctx, action, id, apperr.Invalid(msgMFANotLinked))
		}
		return false, refuse(ctx, action, id, adminError(fmt.Errorf("set mfa: %w", err)))
	}
	if err := s.profiles.SaveMFA(ctx, u, enabled); err != nil {
		return false, refuse(ctx, action, id, apperr.Internal("O MFA foi alterado, mas não foi possível atualizar o registro. Repita a operação.", fmt.Errorf("save mfa: %w", err)))
	}
	audit.Record(ctx, audit.Event{Action: action, Result: audit.Success, Resource: accountResource(id),
		Details: map[string]any{"ativo": enabled}})
	return enabled, nil
}

// Reativação
func (s *Service) EnableAccount(ctx context.Context, id string) error {
	const action = "usuario.reativado"
	u, err := s.account(ctx, id)
	if err != nil {
		return err
	}
	if u.Status == StatusActive {
		return apperr.Conflict(msgAlreadyEnabled)
	}
	if err := s.directory.Enable(ctx, u.Username); err != nil {
		return refuse(ctx, action, id, adminError(err))
	}
	audit.Record(ctx, audit.Event{Action: action, Result: audit.Success, Resource: accountResource(id)})
	return nil
}

// Reenvio
func (s *Service) ResendInvitation(ctx context.Context, id string) error {
	const action = "usuario.convite_reenviado"
	u, err := s.account(ctx, id)
	if err != nil {
		return err
	}
	switch {
	case u.Role == actor.RoleStudent:
		return apperr.Conflict(msgInvitationStaffOnly)
	case u.Status != StatusActive:
		return apperr.Conflict(msgDisabledInvitation)
	case !u.FirstAccessPending:
		return apperr.Conflict(msgNoPendingInvitation)
	}
	if err := s.directory.ResendInvitation(ctx, u.Email); err != nil {
		return refuse(ctx, action, id, adminError(err))
	}
	audit.Record(ctx, audit.Event{Action: action, Result: audit.Success, Resource: accountResource(id)})
	return nil
}

// Criação
func (s *Service) CreateAccount(ctx context.Context, in NewAccount) error {
	name := strings.TrimSpace(in.Name)
	email := normalizeEmail(in.Email)
	switch {
	case name == "" || email == "":
		return apperr.Invalid(msgAccountRequired)
	case !strings.HasSuffix(email, "@umc.br"):

		return apperr.Invalid(msgInstitutionalEmail)
	case in.Role != actor.RoleAdvisor && in.Role != actor.RoleAdmin:
		return apperr.Invalid(msgStaffRole)
	}
	details := map[string]any{"email": email, "perfil": string(in.Role)}
	sub, err := s.directory.CreateAccount(ctx, name, email, in.Role)
	if err != nil {
		var appErr error
		switch {
		case errors.Is(err, errEmailTaken):
			appErr = apperr.Conflict("Já existe uma conta com este e-mail.")
		case errors.Is(err, errInvalidInput):
			appErr = apperr.Invalid("E-mail inválido.")
		default:
			appErr = apperr.Internal("Não foi possível criar a conta. Tente novamente em instantes.", err)
		}
		slog.WarnContext(ctx, "account creation failed", "email", email, "error", err)
		audit.Record(ctx, audit.Event{Action: "usuario.criado", Result: audit.Failure, Reason: message(appErr), Details: details})
		return appErr
	}
	if err := s.profiles.SaveStaff(ctx, sub, name, in.Role); err != nil {
		slog.ErrorContext(ctx, "staff name not saved", "email", email, "error", err)
	}
	audit.Record(ctx, audit.Event{Action: "usuario.criado", Result: audit.Success, Details: details})
	return nil
}

// ---------------------------------------------------------------------------
// Consultas
// ---------------------------------------------------------------------------

// Verificação
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

// Consulta
func (s *Service) StudentNames(ctx context.Context, rgms []string) (map[string]string, error) {
	return s.profiles.StudentNames(ctx, rgms)
}

// Consulta
func (s *Service) AdvisorName(ctx context.Context, sub string) (string, bool, error) {
	if !subFormat.MatchString(sub) {
		return "", false, nil
	}
	p, found, err := s.profiles.Staff(ctx, sub)
	if err != nil || !found || p.Role != actor.RoleAdvisor {
		return "", false, err
	}
	return p.Name, true, nil
}

// Sincronização
func SyncNames(ctx context.Context, d Directory, p Profiles, apply bool) (students, staff int, err error) {
	users, err := d.ListUsers(ctx)
	if err != nil {
		return 0, 0, err
	}
	s := &Service{profiles: p}
	for _, u := range users {
		if u.Role == actor.RoleStudent {
			if u.RGM == "" {
				continue
			}
			students++
		} else {
			staff++
		}
		if apply {
			if err := s.saveName(ctx, u); err != nil {
				return students, staff, err
			}
		}
	}
	return students, staff, nil
}

// Verificação
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
