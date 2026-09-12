package projeto

// Projeto espelha `Projeto` do frontend, com o campo novo OrientadorID
// (ainda não existe no TS — precisa ser adicionado lá também).
type Projeto struct {
	ID           string   `json:"id"`
	Nome         string   `json:"nome"`
	Descricao    string   `json:"descricao"`
	ProgramaID   string   `json:"programaId"`
	Integrantes  []string `json:"integrantes"`
	OrientadorID string   `json:"orientadorId,omitempty"`
}

// NovoProjeto é o corpo esperado em POST /programas/:programaId/projetos.
// ProgramaID vem do path, não do body — evita o mesmo tipo de inconsistência
// que já resolvemos em /auth/registrar (nunca confiar em algo que o
// cliente poderia inventar quando já dá pra derivar de um jeito confiável).
type NovoProjeto struct {
	Nome        string   `json:"nome"`
	Descricao   string   `json:"descricao"`
	Integrantes []string `json:"integrantes"`
}

// AssociarOrientador é o corpo esperado em PUT /projetos/:projetoId/orientador.
type AssociarOrientador struct {
	OrientadorID string `json:"orientadorId"`
}
