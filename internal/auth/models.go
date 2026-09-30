package auth

import "github.com/PFC-Umc-Organization/PFC.Backend/internal/common"

// Perfil é um alias de common.Perfil — fonte única dos papéis, ver
// internal/common/perfil.go. Existe aqui só pra não reescrever o tipo dos
// campos Usuario.Perfil/NovoUsuario.Perfil deste arquivo.
type Perfil = common.Perfil

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

// ConfirmacaoCadastro é o corpo de POST /auth/confirmar — o código de 6
// dígitos que o Cognito manda por e-mail depois do SignUp.
type ConfirmacaoCadastro struct {
	Email  string `json:"email"`
	Codigo string `json:"codigo"`
}

// ReenvioCodigo é o corpo de POST /auth/reenviar-codigo.
type ReenvioCodigo struct {
	Email string `json:"email"`
}
