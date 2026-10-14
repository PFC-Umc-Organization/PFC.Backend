package projeto


type Projeto struct {
	ID           string   `json:"id"`
	Nome         string   `json:"nome"`
	Descricao    string   `json:"descricao"`
	ProgramaID   string   `json:"programaId"`
	Integrantes  []string `json:"integrantes"`
	OrientadorID string   `json:"orientadorId,omitempty"`
}

type NovoProjeto struct {
	Nome        string   `json:"nome"`
	Descricao   string   `json:"descricao"`
	Integrantes []string `json:"integrantes"`
}

type AssociarOrientador struct {
	OrientadorID string `json:"orientadorId"`
}

type AssociarAluno struct {
	RGM string `json:"rgm"`
}

type AtualizarProjeto struct {
	Nome      string `json:"nome"`
	Descricao string `json:"descricao"`
}

// Convite é o pedido pendente pra um aluno entrar no grupo. Sem SES, ele
// fica salvo no sistema e o convidado responde pela tela (aceitar/recusar).
type Convite struct {
	ProjetoID    string `json:"projetoId"`
	ProjetoNome  string `json:"projetoNome"`
	RGM          string `json:"rgm"`
	ConvidadoPor string `json:"convidadoPor"`
	CriadoEm     string `json:"criadoEm"`
}