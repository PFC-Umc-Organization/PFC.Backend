package referencia

import (
	"context"
	"net/url"
	"strings"
)

// Crossref: registro oficial de DOIs. Usado pra consultar UMA obra pelo DOI
// com metadados completos (volume, páginas, editora) — o que a referência
// ABNT precisa.
//
// GET https://api.crossref.org/works/{doi}

type crossrefResposta struct {
	Message crossrefObra `json:"message"`
}

type crossrefObra struct {
	DOI               string   `json:"DOI"`
	Type              string   `json:"type"`
	Title             []string `json:"title"`
	Subtitle          []string `json:"subtitle"`
	ContainerTitle    []string `json:"container-title"`
	Publisher         string   `json:"publisher"`
	PublisherLocation string   `json:"publisher-location"`
	Volume            string   `json:"volume"`
	Issue             string   `json:"issue"`
	Page              string   `json:"page"`
	URL               string   `json:"URL"`
	ReferencedBy      int      `json:"is-referenced-by-count"`
	Author            []struct {
		Given  string `json:"given"`
		Family string `json:"family"`
		Name   string `json:"name"`
	} `json:"author"`
	Issued struct {
		// [[ano, mês, dia]] — o Crossref às vezes manda [[null]].
		DateParts [][]*int `json:"date-parts"`
	} `json:"issued"`
}

func buscarPorDOI(ctx context.Context, doi string) (Obra, error) {
	// Escapa cada parte do DOI, mas mantém a "/" entre registrante e sufixo
	// — é assim que a API espera.
	partes := strings.Split(doi, "/")
	for i, p := range partes {
		partes[i] = url.PathEscape(p)
	}
	endereco := crossrefURL + "/works/" + strings.Join(partes, "/")
	if contatoEmail != "" {
		endereco += "?mailto=" + url.QueryEscape(contatoEmail)
	}

	var resp crossrefResposta
	if err := obterJSON(ctx, endereco, &resp); err != nil {
		return Obra{}, err
	}
	return obraDoCrossref(resp.Message), nil
}

func obraDoCrossref(c crossrefObra) Obra {
	o := Obra{
		DOI:       c.DOI,
		Tipo:      tipoCrossref(c.Type),
		Titulo:    primeiro(c.Title),
		Subtitulo: primeiro(c.Subtitle),
		Veiculo:   primeiro(c.ContainerTitle),
		Editora:   limparTexto(c.Publisher),
		Local:     limparTexto(c.PublisherLocation),
		Volume:    c.Volume,
		Numero:    c.Issue,
		Paginas:   c.Page,
		URL:       c.URL,
		Citacoes:  c.ReferencedBy,
	}

	if len(c.Issued.DateParts) > 0 && len(c.Issued.DateParts[0]) > 0 && c.Issued.DateParts[0][0] != nil {
		o.Ano = *c.Issued.DateParts[0][0]
	}

	for _, a := range c.Author {
		switch {
		case a.Family != "":
			o.Autores = append(o.Autores, Autor{
				Sobrenome: limparTexto(a.Family),
				Prenome:   limparTexto(a.Given),
			})
		case a.Name != "":
			o.Autores = append(o.Autores, Autor{Entidade: limparTexto(a.Name)})
		}
	}

	return o
}

func tipoCrossref(t string) TipoObra {
	switch t {
	case "journal-article":
		return TipoArtigo
	case "proceedings-article":
		return TipoEvento
	case "book-chapter", "book-section", "book-part":
		return TipoCapitulo
	case "book", "monograph", "edited-book", "reference-book":
		return TipoLivro
	default:
		return TipoOutro
	}
}
