package usuario

const (
	PerfilAluno       = "ALUNO"
	PerfilProfessor   = "PROFESSOR"
	PerfilCoordenador = "COORDENADOR"

	StatusAtivo   = "ATIVO"
	StatusInativo = "INATIVO"
)

// Usuario espelha a interface `Usuario` do frontend (usuario.model.ts),
// igual a auth.Usuario, mais o campo Confirmado.
type Usuario struct {
	ID     string `json:"id"`
	Nome   string `json:"nome"`
	Email  string `json:"email,omitempty"`
	Perfil string `json:"perfil"`
	Status string `json:"status"`
	// Confirmado=false: a conta existe, mas o e-mail ainda não foi
	// confirmado — a pessoa ainda não consegue entrar.
	Confirmado bool     `json:"confirmado"`
	CursoIds   []string `json:"cursoIds"`
	RGM        string   `json:"rgm,omitempty"`
}
