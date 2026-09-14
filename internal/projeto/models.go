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