package references

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"
)

var brasilia = time.FixedZone("BRT", -3*60*60)

var abntMonths = [12]string{
	"jan.", "fev.", "mar.", "abr.", "maio", "jun.",
	"jul.", "ago.", "set.", "out.", "nov.", "dez.",
}

const (
	noPlace     = "[S. l.]"
	noPublisher = "[s. n.]"
	noDate      = "[s. d.]"
	noVenue     = "[S. t.]"
)

// ---------------------------------------------------------------------------
// Montagem
// ---------------------------------------------------------------------------

// Trecho
type segment struct {
	text      string
	highlight bool
}

// Montagem
type builder []segment

// Adição
func (b *builder) add(text string) { *b = append(*b, segment{text: text}) }

// Destaque
func (b *builder) highlight(text string) { *b = append(*b, segment{text: text, highlight: true}) }

// Texto
func (b builder) text() string {
	var sb strings.Builder
	for _, s := range b {
		sb.WriteString(s.text)
	}
	return sb.String()
}

// Renderização
func (b builder) html() string {
	var sb strings.Builder
	for _, s := range b {
		if s.highlight {
			sb.WriteString("<strong>" + html.EscapeString(s.text) + "</strong>")
		} else {
			sb.WriteString(html.EscapeString(s.text))
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Formatação
// ---------------------------------------------------------------------------

// Formatação
func formatABNT(w Work, accessed time.Time) (text, htmlText string) {
	var b builder

	authorship := abntAuthorship(w.Authors)
	title := fullTitle(w)
	if authorship == "" {
		title = upperFirstWord(title)
	} else {
		b.add(withPeriod(authorship) + " ")
	}

	switch {
	case w.Type == WorkConferencePaper || w.Type == WorkChapter:
		b.add(withPeriod(title) + " In: ")
		b.highlight(orDefault(w.Venue, noVenue))
		b.add(". " + imprint(w) + ".")
		if w.Pages != "" {
			b.add(" " + pages(w.Pages) + ".")
		}

	case w.Type == WorkArticle || (w.Type == WorkOther && w.Venue != ""):
		b.add(withPeriod(title) + " ")
		b.highlight(orDefault(w.Venue, noVenue))
		details := []string{}
		if w.Volume != "" {
			details = append(details, "v. "+w.Volume)
		}
		if w.Issue != "" {
			details = append(details, "n. "+w.Issue)
		}
		if w.Pages != "" {
			details = append(details, pages(w.Pages))
		}
		details = append(details, year(w.Year))
		b.add(", " + strings.Join(details, ", ") + ".")

	default:
		b.highlight(title)
		b.add(". " + imprint(w) + ".")
	}

	if w.DOI != "" {
		b.add(" DOI: " + w.DOI + ".")
	}
	if link := workLink(w); link != "" {
		b.add(" Disponível em: " + link + ".")
		b.add(" Acesso em: " + abntDate(accessed) + ".")
	}
	return b.text(), b.html()
}

// Formatação
func (a Author) abnt() string {
	if a.Organization != "" {
		return strings.ToUpper(a.Organization)
	}
	if a.GivenName == "" {
		return strings.ToUpper(a.Surname)
	}
	return strings.ToUpper(a.Surname) + ", " + a.GivenName
}

// Formatação
func abntAuthorship(authors []Author) string {
	if len(authors) == 0 {
		return ""
	}
	if len(authors) > 3 {
		return authors[0].abnt() + " et al."
	}
	names := make([]string, 0, len(authors))
	for _, a := range authors {
		names = append(names, a.abnt())
	}
	return strings.Join(names, "; ")
}

// ---------------------------------------------------------------------------
// Auxiliares
// ---------------------------------------------------------------------------

// Composição
func fullTitle(w Work) string {
	if w.Subtitle == "" {
		return w.Title
	}
	return w.Title + ": " + w.Subtitle
}

// Formatação
func imprint(w Work) string {
	return fmt.Sprintf("%s: %s, %s", orDefault(w.Place, noPlace), orDefault(w.Publisher, noPublisher), year(w.Year))
}

// Formatação
func pages(p string) string { return "p. " + strings.ReplaceAll(p, "–", "-") }

// Formatação
func year(y int) string {
	if y == 0 {
		return noDate
	}
	return strconv.Itoa(y)
}

// Formatação
func workLink(w Work) string {
	if w.DOI != "" {
		return "https://doi.org/" + w.DOI
	}
	return w.URL
}

// Formatação
func abntDate(t time.Time) string {
	t = t.In(brasilia)
	return fmt.Sprintf("%d %s %d", t.Day(), abntMonths[t.Month()-1], t.Year())
}

// Formatação
func withPeriod(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasSuffix(s, ".") || strings.HasSuffix(s, "?") || strings.HasSuffix(s, "!") {
		return s
	}
	return s + "."
}

// Capitalização
func upperFirstWord(s string) string {
	word, rest, ok := strings.Cut(s, " ")
	if !ok {
		return strings.ToUpper(word)
	}
	return strings.ToUpper(word) + " " + rest
}

// Valor padrão
func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
