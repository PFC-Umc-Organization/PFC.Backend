package atividade

// TipoCampo é o tipo de um campo do formulário de entrega.
type TipoCampo string

const (
	CampoArquivo    TipoCampo = "ARQUIVO"
	CampoTexto      TipoCampo = "TEXTO"
	CampoTextoLongo TipoCampo = "TEXTO_LONGO"
	CampoLink       TipoCampo = "LINK"
)

// Campo é um campo do formulário que o grupo preenche ao entregar.
type Campo struct {
	ID          string    `json:"id" dynamodbav:"id"`
	Rotulo      string    `json:"rotulo" dynamodbav:"rotulo"`
	Tipo        TipoCampo `json:"tipo" dynamodbav:"tipo"`
	Obrigatorio bool      `json:"obrigatorio" dynamodbav:"obrigatorio"`
}

type NovoCampo struct {
	Rotulo      string    `json:"rotulo"`
	Tipo        TipoCampo `json:"tipo"`
	Obrigatorio bool      `json:"obrigatorio"`
}

// Atividade espelha o tipo Atividade do frontend. Toda atividade tem ao
// menos um campo de entrega.
type Atividade struct {
	ID          string  `json:"id"`
	Titulo      string  `json:"titulo"`
	Descricao   string  `json:"descricao"`
	Prazo       string  `json:"prazo"`
	PublicadaEm string  `json:"publicadaEm"`
	Campos      []Campo `json:"campos"`
}

type NovaAtividade struct {
	Titulo    string `json:"titulo"`
	Descricao string `json:"descricao"`
	Prazo     string `json:"prazo"`
	// Campos é opcional: sem ele a atividade nasce com o campo de arquivo
	// padrão. Informado, precisa ter de 1 a 20 campos.
	Campos []NovoCampo `json:"campos"`
}

type AtualizarAtividade struct {
	Titulo    string `json:"titulo"`
	Descricao string `json:"descricao"`
	Prazo     string `json:"prazo"`
}

// Entrega é a resposta de um projeto (grupo) a uma atividade. Respostas é
// indexado pelo id do campo; em campo ARQUIVO guarda só o nome do arquivo
// (ainda não há armazenamento de arquivos).
type Entrega struct {
	AtividadeID string            `json:"atividadeId"`
	ProjetoID   string            `json:"projetoId"`
	EntregueEm  string            `json:"entregueEm"`
	EntreguePor string            `json:"entreguePor"`
	Respostas   map[string]string `json:"respostas"`
}

type NovaEntrega struct {
	Respostas map[string]string `json:"respostas"`
}
