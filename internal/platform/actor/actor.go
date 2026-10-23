package actor

import "strings"

// Role values are stored in the Cognito attribute custom:perfil.
type Role string

const (
	RoleAdmin   Role = "ADMIN"
	RoleAdvisor Role = "ORIENTADOR"
	RoleStudent Role = "ALUNO"
)

type Actor struct {
	Sub   string
	Name  string
	Email string
	Role  Role
}

func (a Actor) HasRole(roles ...Role) bool {
	for _, r := range roles {
		if a.Role == r {
			return true
		}
	}
	return false
}

// RGM is only meaningful for students, whose e-mail is always
// <rgm>@alunos.umc.br (enforced by the Pre Sign-up Lambda).
func (a Actor) RGM() string { return RGMFromEmail(a.Email) }

func RGMFromEmail(email string) string {
	local, _, ok := strings.Cut(email, "@")
	if !ok {
		return ""
	}
	return local
}

// DisplayName falls back to the e-mail: accounts created in the Cognito
// console may have no name.
func DisplayName(name, email string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return email
}
