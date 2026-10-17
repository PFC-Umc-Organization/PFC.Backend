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
// the e-mail was never confirmed, so the person cannot log in yet.
type User struct {
	ID        string     `json:"id"`
	Name      string     `json:"nome"`
	Email     string     `json:"email,omitempty"`
	Role      actor.Role `json:"perfil"`
	Status    string     `json:"status"`
	Confirmed bool       `json:"confirmado"`
	CohortIDs []string   `json:"cursoIds"`
	RGM       string     `json:"rgm,omitempty"`
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
		Name:      attrs["name"],
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
		Name:      str("name"),
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
	msgAdminOnly           = "acesso restrito a administradores"
	msgCredentialsRequired = "informe o e-mail e a senha"
	msgNotConfirmed        = "conta ainda não confirmada — digite o código enviado por e-mail"
	msgBadCredentials      = "e-mail ou senha inválidos"
	msgSignUpRequired      = "nome, e-mail e senha são obrigatórios"
	msgConfirmRequired     = "informe o e-mail e o código"
	msgEmailRequired       = "informe o e-mail"
	msgAccountRequired     = "nome e email são obrigatórios"
	msgInstitutionalEmail  = "use um e-mail institucional (@umc.br)"
	msgStaffRole           = "perfil deve ser ORIENTADOR ou ADMIN"
	msgThrottled           = "muitas tentativas seguidas — aguarde alguns minutos e tente de novo"
)
