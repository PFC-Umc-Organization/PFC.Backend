package referencia

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// OpenAlex: catálogo aberto de produção acadêmica. Usado pra busca por
// TEMA — o Crossref não tem uma busca textual tão boa, e a OpenAlex ainda
// traz contagem de citações pra ordenar por relevância.
//
// GET https://api.openalex.org/works?search={tema}

const (
	porPagina = 10
	// A OpenAlex só pagina por número até 10.000 resultados; 50 páginas de
	// 10 já é muito mais do que alguém vai rolar numa tela.
	paginaMaxima = 50
)

// Só os campos que a tela usa — a resposta completa da OpenAlex é grande.
const camposOpenAlex = "doi,title,publication_year,cited_by_count,authorships,primary_location"

type openAlexResposta struct {
	Meta struct {
		Count int `json:"count"`
	} `json:"meta"`
	Results []openAlexObra `json:"results"`
}

type openAlexObra struct {
	DOI         *string `json:"doi"`
	Title       *string `json:"title"`
	Year        *int    `json:"publication_year"`
	CitedBy     int     `json:"cited_by_count"`
	Authorships []struct {
		Author struct {
			DisplayName string `json:"display_name"`
		} `json:"author"`
	} `json:"authorships"`
	PrimaryLocation *struct {
		LandingPageURL *string `json:"landing_page_url"`
		Source         *struct {
			DisplayName string `json:"display_name"`
		} `json:"source"`
	} `json:"primary_location"`
}

func buscarPorTema(ctx context.Context, termo string, pagina int) (total int, obras []Obra, err error) {
	params := url.Values{}
	params.Set("search", termo)
	params.Set("page", strconv.Itoa(pagina))
	params.Set("per-page", strconv.Itoa(porPagina))
	params.Set("select", camposOpenAlex)
	if contatoEmail != "" {
		params.Set("mailto", contatoEmail)
	}
	if openAlexAPIKey != "" {
		params.Set("api_key", openAlexAPIKey)
	}

	var resp openAlexResposta
	if err := obterJSON(ctx, openAlexURL+"/works?"+params.Encode(), &resp); err != nil {
		return 0, nil, err
	}

	obras = make([]Obra, 0, len(resp.Results))
	for _, r := range resp.Results {
		obras = append(obras, obraDaOpenAlex(r))
	}
	return resp.Meta.Count, obras, nil
}

func obraDaOpenAlex(r openAlexObra) Obra {
	o := Obra{Tipo: TipoOutro, Citacoes: r.CitedBy}

	if r.DOI != nil {
		// A OpenAlex devolve o DOI como URL (https://doi.org/10...).
		o.DOI, _ = normalizarDOI(*r.DOI)
		o.URL = *r.DOI
	}
	if r.Title != nil {
		o.Titulo = limparTexto(*r.Title)
	}
	if r.Year != nil {
		o.Ano = *r.Year
	}
	if r.PrimaryLocation != nil {
		if r.PrimaryLocation.Source != nil {
			o.Veiculo = limparTexto(r.PrimaryLocation.Source.DisplayName)
		}
		if o.URL == "" && r.PrimaryLocation.LandingPageURL != nil {
			o.URL = *r.PrimaryLocation.LandingPageURL
		}
	}
	for _, a := range r.Authorships {
		if nome := limparTexto(a.Author.DisplayName); nome != "" {
			o.Autores = append(o.Autores, autorDoNomeCompleto(nome))
		}
	}
	return o
}

// Sufixos que, em nomes brasileiros, fazem parte do sobrenome de entrada
// na ABNT: "José Santos Filho" → "SANTOS FILHO, José".
var sufixosParentesco = map[string]bool{
	"filho": true, "filha": true, "neto": true, "neta": true,
	"sobrinho": true, "sobrinha": true, "junior": true, "júnior": true, "jr.": true,
}

// autorDoNomeCompleto separa "Nome Sobrenome" (formato da OpenAlex, que não
// separa os campos como o Crossref). Heurística: o último termo é o
// sobrenome, a não ser que seja um sufixo de parentesco.
func autorDoNomeCompleto(nome string) Autor {
	partes := strings.Fields(nome)
	switch len(partes) {
	case 0:
		return Autor{}
	case 1:
		return Autor{Sobrenome: partes[0]}
	}

	fim := len(partes) - 1
	sobrenome := partes[fim]
	if sufixosParentesco[strings.ToLower(sobrenome)] && fim >= 2 {
		fim--
		sobrenome = partes[fim] + " " + sobrenome
	}
	return Autor{Sobrenome: sobrenome, Prenome: strings.Join(partes[:fim], " ")}
}
