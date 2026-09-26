package referencia

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"
)

// Formatação de referências conforme a ABNT NBR 6023:2018, pros tipos de
// obra que o Crossref devolve. Cobre os casos comuns de TCC — não é um
// formatador completo da norma (editores, edição, tradutor etc. ficam de
// fora porque as APIs raramente trazem esses dados).
//
// Saem duas versões: texto puro e HTML com o destaque em <strong> (a norma
// exige negrito/itálico no título do periódico ou do livro, e o HTML
// preserva isso quando o aluno copia pro Word).

// Brasil não tem mais horário de verão, então UTC-3 fixo evita depender da
// base de fusos (tzdata) no runtime da Lambda.
var fusoBrasilia = time.FixedZone("BRT", -3*60*60)

var mesesABNT = [12]string{
	"jan.", "fev.", "mar.", "abr.", "maio", "jun.",
	"jul.", "ago.", "set.", "out.", "nov.", "dez.",
}

const (
	semLocal   = "[S. l.]"
	semEditora = "[s. n.]"
	// A NBR 6023:2018 pede uma data aproximada entre colchetes quando não
	// há data; como não temos como estimar, fica o "[s. d.]" tradicional.
	semData = "[s. d.]"
)

type trecho struct {
	texto    string
	destaque bool
}

type montador struct {
	trechos []trecho
}

func (m *montador) add(texto string) {
	m.trechos = append(m.trechos, trecho{texto: texto})
}

func (m *montador) destaque(texto string) {
	m.trechos = append(m.trechos, trecho{texto: texto, destaque: true})
}

func (m *montador) texto() string {
	var b strings.Builder
	for _, t := range m.trechos {
		b.WriteString(t.texto)
	}
	return b.String()
}

func (m *montador) html() string {
	var b strings.Builder
	for _, t := range m.trechos {
		if t.destaque {
			b.WriteString("<strong>" + html.EscapeString(t.texto) + "</strong>")
		} else {
			b.WriteString(html.EscapeString(t.texto))
		}
	}
	return b.String()
}

// formatarABNT monta a referência. `acesso` é a data que entra em "Acesso
// em:" — injetada pra os testes não dependerem do relógio.
func formatarABNT(o Obra, acesso time.Time) (texto, htmlFormatado string) {
	m := &montador{}

	autoria := autoriaABNT(o.Autores)
	titulo := tituloCompleto(o)
	if autoria == "" {
		// Sem autor, a entrada é pelo título, com a primeira palavra em
		// maiúsculas.
		titulo = primeiraPalavraMaiuscula(titulo)
	} else {
		m.add(comPonto(autoria) + " ")
	}

	switch {
	case o.Tipo == TipoEvento || o.Tipo == TipoCapitulo:
		// AUTOR. Título. In: Anais/Livro. Local: Editora, ano. p. x-y.
		m.add(comPonto(titulo) + " In: ")
		m.destaque(valorOu(o.Veiculo, "[S. t.]"))
		m.add(". " + imprenta(o) + ".")
		if o.Paginas != "" {
			m.add(" " + paginas(o.Paginas) + ".")
		}

	case o.Tipo == TipoArtigo || (o.Tipo == TipoOutro && o.Veiculo != ""):
		// AUTOR. Título do artigo. Periódico, v. x, n. y, p. x-y, ano.
		m.add(comPonto(titulo) + " ")
		m.destaque(valorOu(o.Veiculo, "[S. t.]"))
		detalhes := []string{}
		if o.Volume != "" {
			detalhes = append(detalhes, "v. "+o.Volume)
		}
		if o.Numero != "" {
			detalhes = append(detalhes, "n. "+o.Numero)
		}
		if o.Paginas != "" {
			detalhes = append(detalhes, paginas(o.Paginas))
		}
		detalhes = append(detalhes, ano(o.Ano))
		m.add(", " + strings.Join(detalhes, ", ") + ".")

	default:
		// Livro (e o que mais não tiver veículo): título em destaque.
		// AUTOR. Título. Local: Editora, ano.
		m.destaque(titulo)
		m.add(". " + imprenta(o) + ".")
	}

	if o.DOI != "" {
		m.add(" DOI: " + o.DOI + ".")
	}
	if link := linkDaObra(o); link != "" {
		m.add(" Disponível em: " + link + ".")
		m.add(" Acesso em: " + dataABNT(acesso) + ".")
	}

	return m.texto(), m.html()
}

// abnt formata um autor como entrada: "SOBRENOME, Prenome".
func (a Autor) abnt() string {
	if a.Entidade != "" {
		return strings.ToUpper(a.Entidade)
	}
	if a.Prenome == "" {
		return strings.ToUpper(a.Sobrenome)
	}
	return strings.ToUpper(a.Sobrenome) + ", " + a.Prenome
}

// autoriaABNT: até três autores, todos separados por "; ". A partir de
// quatro, a norma permite indicar só o primeiro seguido de "et al." — e é o
// que evita referências de meia página em artigos com 20 autores.
func autoriaABNT(autores []Autor) string {
	if len(autores) == 0 {
		return ""
	}
	if len(autores) > 3 {
		return autores[0].abnt() + " et al."
	}
	nomes := make([]string, 0, len(autores))
	for _, a := range autores {
		nomes = append(nomes, a.abnt())
	}
	return strings.Join(nomes, "; ")
}

func tituloCompleto(o Obra) string {
	if o.Subtitulo == "" {
		return o.Titulo
	}
	return o.Titulo + ": " + o.Subtitulo
}

// imprenta é o "Local: Editora, ano" das referências de livro e evento.
func imprenta(o Obra) string {
	return fmt.Sprintf("%s: %s, %s",
		valorOu(o.Local, semLocal),
		valorOu(o.Editora, semEditora),
		ano(o.Ano))
}

func paginas(p string) string {
	return "p. " + strings.ReplaceAll(p, "–", "-")
}

func ano(a int) string {
	if a == 0 {
		return semData
	}
	return strconv.Itoa(a)
}

func linkDaObra(o Obra) string {
	if o.DOI != "" {
		return "https://doi.org/" + o.DOI
	}
	return o.URL
}

func dataABNT(t time.Time) string {
	t = t.In(fusoBrasilia)
	return fmt.Sprintf("%d %s %d", t.Day(), mesesABNT[t.Month()-1], t.Year())
}

// comPonto fecha o elemento com ponto, sem duplicar quando ele já termina
// em ponto (ex: "et al.", prenome abreviado "J.") ou em ?/!.
func comPonto(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasSuffix(s, ".") || strings.HasSuffix(s, "?") || strings.HasSuffix(s, "!") {
		return s
	}
	return s + "."
}

func primeiraPalavraMaiuscula(s string) string {
	palavra, resto, temResto := strings.Cut(s, " ")
	if !temResto {
		return strings.ToUpper(palavra)
	}
	return strings.ToUpper(palavra) + " " + resto
}

func valorOu(valor, padrao string) string {
	if strings.TrimSpace(valor) == "" {
		return padrao
	}
	return valor
}
