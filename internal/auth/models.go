package auth

// Perfil espelha o union type `Perfil` do frontend (usuario.model.ts).
type Perfil string

const (
	PerfilAluno       Perfil = "ALUNO"
	PerfilProfessor   Perfil = "PROFESSOR"
	PerfilCoordenador Perfil = "COORDENADOR"
)

// StatusUsuario espelha `StatusUsuario` do frontend.
type StatusUsuario string

const (
	StatusAtivo   StatusUsuario = "ATIVO"
	StatusInativo StatusUsuario = "INATIVO"
)

// Usuario espelha a interface `Usuario` do frontend — os nomes de campo em
// JSON têm que bater exatamente (camelCase, cursoIds) porque o Angular
// desserializa isso direto num objeto TS sem camada de tradução no meio.
type Usuario struct {
	ID       string        `json:"id"`
	Nome     string        `json:"nome"`
	Email    string        `json:"email"`
	Perfil   Perfil        `json:"perfil"`
	Status   StatusUsuario `json:"status"`
	CursoIds []string      `json:"cursoIds"`
	// RGM só existe pra aluno — vem do e-mail (<rgm>@alunos.umc.br).
	RGM string `json:"rgm,omitempty"`
}

// Credenciais espelha `Credenciais` — corpo esperado em POST /auth/login.
type Credenciais struct {
	Email string `json:"email"`
	Senha string `json:"senha"`
}

// NovoUsuario espelha `NovoUsuario` — corpo esperado em POST /auth/registrar.
//
// ATENÇÃO: o campo Perfil existe no contrato porque o frontend tem essa
// opção no formulário de cadastro, mas o handler NUNCA deve confiar nesse
// valor pra decidir o perfil real do usuário criado — ver comentário em
// handler.go. Ele é lido aqui só pra não quebrar o unmarshal do JSON.
type NovoUsuario struct {
	Nome   string `json:"nome"`
	Email  string `json:"email"`
	Senha  string `json:"senha"`
	Perfil Perfil `json:"perfil"`
}

// RespostaAuth é o formato de resposta de login/cadastro: usuário + token,
// conforme documentado no README do frontend ("devolve usuário + token").
type RespostaAuth struct {
	Usuario Usuario `json:"usuario"`
	Token   string  `json:"token"`
}
