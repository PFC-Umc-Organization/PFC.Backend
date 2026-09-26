package referencia

// TipoObra decide o layout da referência ABNT (NBR 6023).
type TipoObra string

const (
	TipoArtigo   TipoObra = "ARTIGO"   // artigo de periódico
	TipoEvento   TipoObra = "EVENTO"   // trabalho publicado em anais de evento
	TipoCapitulo TipoObra = "CAPITULO" // capítulo de livro
	TipoLivro    TipoObra = "LIVRO"
	TipoOutro    TipoObra = "OUTRO"
)

// Autor de uma obra. Entidade é preenchida quando a autoria é de uma
// organização (ex: "World Health Organization") — nesse caso não há
// sobrenome/prenome.
type Autor struct {
	Sobrenome string
	Prenome   string
	Entidade  string
}

// Obra é a representação interna dos metadados, independente da API de
// origem (Crossref ou OpenAlex). É daqui que sai a referência ABNT.
type Obra struct {
	DOI       string
	Tipo      TipoObra
	Titulo    string
	Subtitulo string
	Autores   []Autor
	// Veiculo é o periódico (artigo), o nome dos anais (evento) ou o livro
	// que contém o capítulo.
	Veiculo  string
	Editora  string
	Local    string
	Volume   string
	Numero   string
	Paginas  string
	Ano      int
	URL      string
	Citacoes int
}

// ArtigoEncontrado é o item da busca devolvido ao frontend.
type ArtigoEncontrado struct {
	DOI      string   `json:"doi,omitempty"`
	Titulo   string   `json:"titulo"`
	Autores  []string `json:"autores"`
	Ano      int      `json:"ano,omitempty"`
	Veiculo  string   `json:"veiculo,omitempty"`
	Citacoes int      `json:"citacoes"`
	URL      string   `json:"url,omitempty"`
}

// ResultadoBusca é a resposta de GET /referencias/busca.
type ResultadoBusca struct {
	Total   int                `json:"total"`
	Pagina  int                `json:"pagina"`
	Artigos []ArtigoEncontrado `json:"artigos"`
}

// ReferenciaFormatada é a resposta de GET /referencias/doi — pré-visualização
// antes de o aluno adicionar à lista do projeto.
type ReferenciaFormatada struct {
	ArtigoEncontrado
	ABNT     string `json:"abnt"`
	ABNTHTML string `json:"abntHtml"`
}

// Referencia é um item salvo na lista de referências de um projeto.
type Referencia struct {
	ID            string `json:"id"`
	ProjetoID     string `json:"projetoId"`
	DOI           string `json:"doi"`
	Titulo        string `json:"titulo"`
	Ano           int    `json:"ano,omitempty"`
	ABNT          string `json:"abnt"`
	ABNTHTML      string `json:"abntHtml"`
	AdicionadaPor string `json:"adicionadaPor"`
	AdicionadaEm  string `json:"adicionadaEm"`
}

// NovaReferencia é o corpo de POST /projetos/:projetoId/referencias. Só o
// DOI — os metadados são buscados de novo no Crossref, pra ninguém gravar
// referência com dados inventados.
type NovaReferencia struct {
	DOI string `json:"doi"`
}

func paraArtigo(o Obra) ArtigoEncontrado {
	autores := make([]string, 0, len(o.Autores))
	for _, a := range o.Autores {
		autores = append(autores, a.abnt())
	}
	return ArtigoEncontrado{
		DOI:      o.DOI,
		Titulo:   tituloCompleto(o),
		Autores:  autores,
		Ano:      o.Ano,
		Veiculo:  o.Veiculo,
		Citacoes: o.Citacoes,
		URL:      o.URL,
	}
}
