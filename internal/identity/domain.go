// Package identity owns accounts, which live only in Cognito: login,
// student self sign-up and confirmation, the account list and the accounts
// an admin creates for advisors and admins.
package identity

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
)

const (
	StatusActive   = "ATIVO"
	StatusInactive = "INATIVO"
)

// User is an account as the account list shows it. Confirmed=false means
// the e-mail was never confirmed, so the person cannot log in yet;
// FirstAccessPending, that a staff account still has its temporary password.
// Username is Cognito's, which the admin calls take.
type User struct {
	ID                 string     `json:"id"`
	Name               string     `json:"nome"`
	Email              string     `json:"email,omitempty"`
	Role               actor.Role `json:"perfil"`
	Status             string     `json:"status"`
	Confirmed          bool       `json:"confirmado"`
	FirstAccessPending bool       `json:"aguardandoPrimeiroAcesso,omitempty"`
	CohortIDs          []string   `json:"cursoIds"`
	RGM                string     `json:"rgm,omitempty"`
	Username           string     `json:"-"`
}

type SessionUser struct {
	ID        string     `json:"id"`
	Name      string     `json:"nome"`
	Email     string     `json:"email"`
	Role      actor.Role `json:"perfil"`
	Status    string     `json:"status"`
	CohortIDs []string   `json:"cursoIds"`
	RGM       string     `json:"rgm,omitempty"`
}

type Session struct {
	User  SessionUser `json:"usuario"`
	Token string      `json:"token"`
}

// LoginResult is a Session, or — for an account created by an admin, whose
// temporary password must be replaced on the first login — the challenge the
// frontend answers through POST /auth/nova-senha.
type LoginResult struct {
	User      *SessionUser `json:"usuario,omitempty"`
	Token     string       `json:"token,omitempty"`
	Challenge string       `json:"desafio,omitempty"`
	Session   string       `json:"sessao,omitempty"`
}

const ChallengeNewPassword = "NOVA_SENHA"

type NewPasswordInput struct {
	Email    string `json:"email"`
	Session  string `json:"sessao"`
	Password string `json:"senha"`
}

type ForgotPasswordInput struct {
	Email string `json:"email"`
}

type ResetPasswordInput struct {
	Email    string `json:"email"`
	Code     string `json:"codigo"`
	Password string `json:"senha"`
}

type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"senha"`
}

// SignUpInput carries a role because the frontend form has one; it is
// never trusted: public sign-up always creates a student.
type SignUpInput struct {
	Name     string     `json:"nome"`
	Email    string     `json:"email"`
	Password string     `json:"senha"`
	Role     actor.Role `json:"perfil"`
}

type Confirmation struct {
	Email string `json:"email"`
	Code  string `json:"codigo"`
}

type ResendInput struct {
	Email string `json:"email"`
}

type NameInput struct {
	Name string `json:"nome"`
}

type RoleInput struct {
	Role actor.Role `json:"perfil"`
}

const maxAccountNameLen = 150

// NewAccount is for advisors and admins; students only sign themselves up.
type NewAccount struct {
	Name  string     `json:"nome"`
	Email string     `json:"email"`
	Role  actor.Role `json:"perfil"`
}

func normalizeEmail(e string) string { return strings.TrimSpace(strings.ToLower(e)) }

// roleOrStudent: self-registered accounts may carry no custom:perfil.
func roleOrStudent(r string) actor.Role {
	if r == "" {
		return actor.RoleStudent
	}
	return actor.Role(r)
}

// With the e-mail as username, the Cognito username is the sub.
func userFromAttributes(attrs map[string]string, username string, enabled, confirmed bool) User {
	id := attrs["sub"]
	if id == "" {
		id = username
	}
	status := StatusActive
	if !enabled {
		status = StatusInactive
	}
	u := User{
		ID:        id,
		Username:  username,
		Name:      actor.DisplayName(attrs["name"], attrs["email"]),
		Email:     attrs["email"],
		Role:      roleOrStudent(attrs["custom:perfil"]),
		Status:    status,
		Confirmed: confirmed,
		CohortIDs: []string{},
	}
	if u.Role == actor.RoleStudent {
		u.RGM = actor.RGMFromEmail(u.Email)
	}
	return u
}

// claimsOf decodes the ID token payload without checking the signature:
// the token was issued by Cognito in this same request. Signatures matter
// for tokens coming from outside, which the API Gateway authorizer checks.
func claimsOf(idToken string) (map[string]any, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func sessionUser(claims map[string]any) SessionUser {
	str := func(k string) string { s, _ := claims[k].(string); return s }
	u := SessionUser{
		ID:        str("sub"),
		Name:      actor.DisplayName(str("name"), str("email")),
		Email:     str("email"),
		Role:      roleOrStudent(str("custom:perfil")),
		Status:    StatusActive,
		CohortIDs: []string{},
	}
	if u.Role == actor.RoleStudent {
		u.RGM = actor.RGMFromEmail(u.Email)
	}
	return u
}

var (
	// Both values go into a Cognito ListUsers filter: only their exact
	// formats are accepted, so nobody can build another filter.
	digitsOnly = regexp.MustCompile(`^\d+$`)
	subFormat  = regexp.MustCompile(`^[0-9a-fA-F-]{36}$`)
)

const (
	msgAdminOnly           = "Acesso restrito à coordenação."
	msgStaffOnly           = "Acesso restrito a orientadores e à coordenação."
	msgCredentialsRequired = "Informe o e-mail e a senha."
	msgNotConfirmed        = "Conta ainda não confirmada. Digite o código enviado por e-mail."
	msgBadCredentials      = "E-mail ou senha incorretos."
	msgSignUpRequired      = "Informe o nome, o e-mail e a senha."
	msgConfirmRequired     = "Informe o e-mail e o código."
	msgEmailRequired       = "Informe o e-mail."
	msgAccountRequired     = "Informe o nome e o e-mail."
	msgInstitutionalEmail  = "Use um e-mail institucional (@umc.br)."
	msgStaffRole           = "Selecione o perfil: orientador ou coordenação."
	msgThrottled           = "Muitas tentativas em sequência. Aguarde alguns minutos e tente novamente."
	msgNewPasswordRequired = "Dados incompletos. Entre novamente com a senha temporária e defina a nova senha."
	msgResetRequired       = "Informe o e-mail, o código e a nova senha."
	msgWeakPassword        = "A senha precisa ter ao menos 8 caracteres, com letra maiúscula, letra minúscula, número e símbolo."
	msgSessionExpired      = "O prazo para definir a senha expirou. Entre novamente com a senha temporária recebida por e-mail."
	msgWrongCode           = "Código incorreto. Confira o código recebido por e-mail e tente novamente."
	msgExpiredCode         = "Código expirado. Solicite um novo código."
	msgConfirmFailed       = "Código incorreto. Confira o e-mail e o código informados. Se a conta já foi confirmada, basta entrar."
	msgNoSession           = "Sessão expirada. Entre novamente."

	msgAccountNotFound     = "Conta não encontrada."
	msgNameRequired        = "Informe o nome."
	msgNameTooLong         = "O nome excede o tamanho máximo permitido."
	msgStudentRole         = "Contas de aluno não mudam de perfil."
	msgSameRole            = "A conta já tem este perfil."
	msgOwnRole             = "Você não pode alterar o próprio perfil. Peça a outra pessoa da coordenação."
	msgOwnDisable          = "Você não pode desativar a própria conta. Peça a outra pessoa da coordenação."
	msgLastAdmin           = "Esta é a única conta ativa da coordenação. Promova outra conta à coordenação antes de alterá-la."
	msgAlreadyDisabled     = "A conta já está desativada."
	msgAlreadyEnabled      = "A conta já está ativa."
	msgNoPendingInvitation = "Esta conta já fez o primeiro acesso. Para recuperar a senha, a pessoa pode usar \"Esqueci minha senha\" na tela de entrada."
	msgInvitationStaffOnly = "Só contas de orientador e da coordenação recebem convite. Alunos se cadastram pela tela de entrada."
	msgDisabledInvitation  = "Reative a conta antes de reenviar o convite."
)
