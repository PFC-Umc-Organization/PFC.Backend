package actor

import "strings"

// Enumeração
type Role string

const (
	RoleAdmin   Role = "ADMIN"
	RoleAdvisor Role = "ORIENTADOR"
	RoleStudent Role = "ALUNO"
)

// Ator
type Actor struct {
	Sub   string
	Name  string
	Email string
	Role  Role
}

// Verificação
func (a Actor) HasRole(roles ...Role) bool {
	for _, r := range roles {
		if a.Role == r {
			return true
		}
	}
	return false
}

// Extração
func (a Actor) RGM() string { return RGMFromEmail(a.Email) }

// Extração
func RGMFromEmail(email string) string {
	local, _, ok := strings.Cut(email, "@")
	if !ok {
		return ""
	}
	return local
}

// Exibição
func DisplayName(name, email string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return email
}
