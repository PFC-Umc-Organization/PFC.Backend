package references

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"
)

// ABNT NBR 6023:2018 for the work types Crossref returns. It covers the
// usual cases of a final paper, not the whole standard: editors, edition
// and translators are left out because the APIs rarely provide them.
//
// The HTML version marks the highlighted element with <strong>, which the
// standard requires and which survives pasting into Word.

// Brazil no longer has daylight saving time; a fixed UTC-3 avoids depending
// on tzdata in the Lambda runtime.
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

type segment struct {
	text      string
	highlight bool
}

type builder []segment

func (b *builder) add(text string)       { *b = append(*b, segment{text: text}) }
func (b *builder) highlight(text string) { *b = append(*b, segment{text: text, highlight: true}) }

func (b builder) text() string {
	var sb strings.Builder
	for _, s := range b {
		sb.WriteString(s.text)
	}
	return sb.String()
}

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

// formatABNT returns the reference as text and HTML; accessed is the date
// in "Acesso em:".
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

func (a Author) abnt() string {
	if a.Organization != "" {
		return strings.ToUpper(a.Organization)
	}
	if a.GivenName == "" {
		return strings.ToUpper(a.Surname)
	}
	return strings.ToUpper(a.Surname) + ", " + a.GivenName
}

// abntAuthorship lists up to three authors; from four on the standard
// allows the first one followed by "et al.".
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

func fullTitle(w Work) string {
	if w.Subtitle == "" {
		return w.Title
	}
	return w.Title + ": " + w.Subtitle
}

func imprint(w Work) string {
	return fmt.Sprintf("%s: %s, %s", orDefault(w.Place, noPlace), orDefault(w.Publisher, noPublisher), year(w.Year))
}

func pages(p string) string { return "p. " + strings.ReplaceAll(p, "–", "-") }

func year(y int) string {
	if y == 0 {
		return noDate
	}
	return strconv.Itoa(y)
}

func workLink(w Work) string {
	if w.DOI != "" {
		return "https://doi.org/" + w.DOI
	}
	return w.URL
}

func abntDate(t time.Time) string {
	t = t.In(brasilia)
	return fmt.Sprintf("%d %s %d", t.Day(), abntMonths[t.Month()-1], t.Year())
}

// withPeriod does not double a period after "et al." or an initial, nor
// add one after ? or !.
func withPeriod(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasSuffix(s, ".") || strings.HasSuffix(s, "?") || strings.HasSuffix(s, "!") {
		return s
	}
	return s + "."
}

func upperFirstWord(s string) string {
	word, rest, ok := strings.Cut(s, " ")
	if !ok {
		return strings.ToUpper(word)
	}
	return strings.ToUpper(word) + " " + rest
}

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
