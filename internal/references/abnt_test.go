package references

import (
	"testing"
	"time"
)

// 26/09/2026 12:00 in Brasília.
var accessDate = time.Date(2026, 9, 26, 15, 0, 0, 0, time.UTC)

func TestFormatABNT(t *testing.T) {
	cases := []struct {
		name string
		work Work
		text string
		html string
	}{
		{
			name: "conference paper with more than 3 authors uses et al.",
			work: Work{
				DOI:  "10.1145/3290605.3300233",
				Type: WorkConferencePaper,
				Authors: []Author{
					{Surname: "Amershi", GivenName: "Saleema"},
					{Surname: "Weld", GivenName: "Dan"},
					{Surname: "Vorvoreanu", GivenName: "Mihaela"},
					{Surname: "Fourney", GivenName: "Adam"},
				},
				Title:     "Guidelines for Human-AI Interaction",
				Venue:     "Proceedings of the 2019 CHI Conference on Human Factors in Computing Systems",
				Publisher: "ACM",
				Place:     "New York, NY, USA",
				Pages:     "1-13",
				Year:      2019,
			},
			text: "AMERSHI, Saleema et al. Guidelines for Human-AI Interaction. In: " +
				"Proceedings of the 2019 CHI Conference on Human Factors in Computing Systems. " +
				"New York, NY, USA: ACM, 2019. p. 1-13. DOI: 10.1145/3290605.3300233. " +
				"Disponível em: https://doi.org/10.1145/3290605.3300233. Acesso em: 26 set. 2026.",
			html: "AMERSHI, Saleema et al. Guidelines for Human-AI Interaction. In: " +
				"<strong>Proceedings of the 2019 CHI Conference on Human Factors in Computing Systems</strong>. " +
				"New York, NY, USA: ACM, 2019. p. 1-13. DOI: 10.1145/3290605.3300233. " +
				"Disponível em: https://doi.org/10.1145/3290605.3300233. Acesso em: 26 set. 2026.",
		},
		{
			name: "journal article with subtitle, volume, issue and pages",
			work: Work{
				Type: WorkArticle,
				Authors: []Author{
					{Surname: "Silva", GivenName: "Ana"},
					{Surname: "Souza", GivenName: "Carlos"},
				},
				Title:    "Gestão de projetos",
				Subtitle: "um estudo de caso",
				Venue:    "Revista de Administração",
				Volume:   "16",
				Issue:    "3",
				Pages:    "283–300", // en dash, as Crossref sometimes sends
				Year:     2015,
			},
			text: "SILVA, Ana; SOUZA, Carlos. Gestão de projetos: um estudo de caso. " +
				"Revista de Administração, v. 16, n. 3, p. 283-300, 2015.",
			html: "SILVA, Ana; SOUZA, Carlos. Gestão de projetos: um estudo de caso. " +
				"<strong>Revista de Administração</strong>, v. 16, n. 3, p. 283-300, 2015.",
		},
		{
			name: "book without place, publisher and date uses the standard's markers",
			work: Work{
				Type:    WorkBook,
				Authors: []Author{{Surname: "Knuth", GivenName: "Donald"}},
				Title:   "The Art of Computer Programming",
				URL:     "https://exemplo.org/livro",
			},
			text: "KNUTH, Donald. The Art of Computer Programming. [S. l.]: [s. n.], [s. d.]. " +
				"Disponível em: https://exemplo.org/livro. Acesso em: 26 set. 2026.",
			html: "KNUTH, Donald. <strong>The Art of Computer Programming</strong>. [S. l.]: [s. n.], [s. d.]. " +
				"Disponível em: https://exemplo.org/livro. Acesso em: 26 set. 2026.",
		},
		{
			name: "without author the entry is the title's first word",
			work: Work{
				Type:  WorkArticle,
				Title: "Relatório anual de pesquisa",
				Venue: "Boletim",
				Year:  2020,
			},
			text: "RELATÓRIO anual de pesquisa. Boletim, 2020.",
			html: "RELATÓRIO anual de pesquisa. <strong>Boletim</strong>, 2020.",
		},
		{
			name: "organization as author",
			work: Work{
				Type:      WorkBook,
				Authors:   []Author{{Organization: "World Health Organization"}},
				Title:     "World report on ageing and health",
				Publisher: "WHO",
				Place:     "Geneva",
				Year:      2015,
			},
			text: "WORLD HEALTH ORGANIZATION. World report on ageing and health. Geneva: WHO, 2015.",
			html: "WORLD HEALTH ORGANIZATION. <strong>World report on ageing and health</strong>. Geneva: WHO, 2015.",
		},
		{
			name: "abbreviated given name does not double the period",
			work: Work{
				Type:    WorkArticle,
				Authors: []Author{{Surname: "Doe", GivenName: "J."}},
				Title:   "Um título",
				Venue:   "Periódico",
				Year:    2021,
			},
			text: "DOE, J. Um título. Periódico, 2021.",
			html: "DOE, J. Um título. <strong>Periódico</strong>, 2021.",
		},
		{
			name: "HTML is escaped",
			work: Work{
				Type:    WorkArticle,
				Authors: []Author{{Surname: "Lee", GivenName: "Kim"}},
				Title:   "Tags <script> & outros",
				Venue:   "Science & Society",
				Year:    2022,
			},
			text: "LEE, Kim. Tags <script> & outros. Science & Society, 2022.",
			html: "LEE, Kim. Tags &lt;script&gt; &amp; outros. <strong>Science &amp; Society</strong>, 2022.",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, html := formatABNT(c.work, accessDate)
			if text != c.text {
				t.Errorf("text\n got: %s\nwant: %s", text, c.text)
			}
			if html != c.html {
				t.Errorf("html\n got: %s\nwant: %s", html, c.html)
			}
		})
	}
}

func TestABNTAuthorship(t *testing.T) {
	three := []Author{
		{Surname: "Silva", GivenName: "Ana"},
		{Surname: "Santos Filho", GivenName: "José"},
		{Surname: "Lima", GivenName: "Rui"},
	}
	if got, want := abntAuthorship(three), "SILVA, Ana; SANTOS FILHO, José; LIMA, Rui"; got != want {
		t.Errorf("three authors: got %q, want %q", got, want)
	}
	if got := abntAuthorship(nil); got != "" {
		t.Errorf("no authors: got %q, want empty", got)
	}
}

func TestABNTDate(t *testing.T) {
	cases := map[string]time.Time{
		"5 maio 2026": time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC),
		// 02:00 UTC on the 1st is still the 30th in Brasília.
		"30 set. 2026": time.Date(2026, 10, 1, 2, 0, 0, 0, time.UTC),
		"1 jan. 2027":  time.Date(2027, 1, 1, 15, 0, 0, 0, time.UTC),
	}
	for want, date := range cases {
		if got := abntDate(date); got != want {
			t.Errorf("abntDate(%s) = %q, want %q", date, got, want)
		}
	}
}

func TestAuthorFromFullName(t *testing.T) {
	cases := []struct {
		name string
		want Author
	}{
		{"Saleema Amershi", Author{Surname: "Amershi", GivenName: "Saleema"}},
		{"João da Silva", Author{Surname: "Silva", GivenName: "João da"}},
		{"José Santos Filho", Author{Surname: "Santos Filho", GivenName: "José"}},
		{"Carlos Lima Júnior", Author{Surname: "Lima Júnior", GivenName: "Carlos"}},
		{"Neto Filho", Author{Surname: "Filho", GivenName: "Neto"}},
		{"Madonna", Author{Surname: "Madonna"}},
		{"  ", Author{}},
	}
	for _, c := range cases {
		if got := authorFromFullName(c.name); got != c.want {
			t.Errorf("authorFromFullName(%q) = %+v, want %+v", c.name, got, c.want)
		}
	}
}
