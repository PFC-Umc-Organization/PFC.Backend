package referencia

import (
	"testing"
	"time"
)

// 26/09/2026 12:00 em Brasília.
var dataAcesso = time.Date(2026, 9, 26, 15, 0, 0, 0, time.UTC)

func TestFormatarABNT(t *testing.T) {
	casos := []struct {
		nome      string
		obra      Obra
		texto     string
		htmlFinal string
	}{
		{
			nome: "trabalho em evento com mais de 3 autores usa et al.",
			obra: Obra{
				DOI:  "10.1145/3290605.3300233",
				Tipo: TipoEvento,
				Autores: []Autor{
					{Sobrenome: "Amershi", Prenome: "Saleema"},
					{Sobrenome: "Weld", Prenome: "Dan"},
					{Sobrenome: "Vorvoreanu", Prenome: "Mihaela"},
					{Sobrenome: "Fourney", Prenome: "Adam"},
				},
				Titulo:  "Guidelines for Human-AI Interaction",
				Veiculo: "Proceedings of the 2019 CHI Conference on Human Factors in Computing Systems",
				Editora: "ACM",
				Local:   "New York, NY, USA",
				Paginas: "1-13",
				Ano:     2019,
			},
			texto: "AMERSHI, Saleema et al. Guidelines for Human-AI Interaction. In: " +
				"Proceedings of the 2019 CHI Conference on Human Factors in Computing Systems. " +
				"New York, NY, USA: ACM, 2019. p. 1-13. DOI: 10.1145/3290605.3300233. " +
				"Disponível em: https://doi.org/10.1145/3290605.3300233. Acesso em: 26 set. 2026.",
			htmlFinal: "AMERSHI, Saleema et al. Guidelines for Human-AI Interaction. In: " +
				"<strong>Proceedings of the 2019 CHI Conference on Human Factors in Computing Systems</strong>. " +
				"New York, NY, USA: ACM, 2019. p. 1-13. DOI: 10.1145/3290605.3300233. " +
				"Disponível em: https://doi.org/10.1145/3290605.3300233. Acesso em: 26 set. 2026.",
		},
		{
			nome: "artigo de periódico com subtítulo, volume, número e páginas",
			obra: Obra{
				Tipo: TipoArtigo,
				Autores: []Autor{
					{Sobrenome: "Silva", Prenome: "Ana"},
					{Sobrenome: "Souza", Prenome: "Carlos"},
				},
				Titulo:    "Gestão de projetos",
				Subtitulo: "um estudo de caso",
				Veiculo:   "Revista de Administração",
				Volume:    "16",
				Numero:    "3",
				Paginas:   "283–300", // travessão, como o Crossref às vezes manda
				Ano:       2015,
			},
			texto: "SILVA, Ana; SOUZA, Carlos. Gestão de projetos: um estudo de caso. " +
				"Revista de Administração, v. 16, n. 3, p. 283-300, 2015.",
			htmlFinal: "SILVA, Ana; SOUZA, Carlos. Gestão de projetos: um estudo de caso. " +
				"<strong>Revista de Administração</strong>, v. 16, n. 3, p. 283-300, 2015.",
		},
		{
			nome: "livro sem local, editora e data usa os marcadores da norma",
			obra: Obra{
				Tipo:    TipoLivro,
				Autores: []Autor{{Sobrenome: "Knuth", Prenome: "Donald"}},
				Titulo:  "The Art of Computer Programming",
				URL:     "https://exemplo.org/livro",
			},
			texto: "KNUTH, Donald. The Art of Computer Programming. [S. l.]: [s. n.], [s. d.]. " +
				"Disponível em: https://exemplo.org/livro. Acesso em: 26 set. 2026.",
			htmlFinal: "KNUTH, Donald. <strong>The Art of Computer Programming</strong>. [S. l.]: [s. n.], [s. d.]. " +
				"Disponível em: https://exemplo.org/livro. Acesso em: 26 set. 2026.",
		},
		{
			nome: "sem autor, a entrada é pela primeira palavra do título",
			obra: Obra{
				Tipo:    TipoArtigo,
				Titulo:  "Relatório anual de pesquisa",
				Veiculo: "Boletim",
				Ano:     2020,
			},
			texto:     "RELATÓRIO anual de pesquisa. Boletim, 2020.",
			htmlFinal: "RELATÓRIO anual de pesquisa. <strong>Boletim</strong>, 2020.",
		},
		{
			nome: "autoria de entidade",
			obra: Obra{
				Tipo:    TipoLivro,
				Autores: []Autor{{Entidade: "World Health Organization"}},
				Titulo:  "World report on ageing and health",
				Editora: "WHO",
				Local:   "Geneva",
				Ano:     2015,
			},
			texto:     "WORLD HEALTH ORGANIZATION. World report on ageing and health. Geneva: WHO, 2015.",
			htmlFinal: "WORLD HEALTH ORGANIZATION. <strong>World report on ageing and health</strong>. Geneva: WHO, 2015.",
		},
		{
			nome: "prenome abreviado não duplica o ponto",
			obra: Obra{
				Tipo:    TipoArtigo,
				Autores: []Autor{{Sobrenome: "Doe", Prenome: "J."}},
				Titulo:  "Um título",
				Veiculo: "Periódico",
				Ano:     2021,
			},
			texto:     "DOE, J. Um título. Periódico, 2021.",
			htmlFinal: "DOE, J. Um título. <strong>Periódico</strong>, 2021.",
		},
		{
			nome: "HTML é escapado",
			obra: Obra{
				Tipo:    TipoArtigo,
				Autores: []Autor{{Sobrenome: "Lee", Prenome: "Kim"}},
				Titulo:  "Tags <script> & outros",
				Veiculo: "Science & Society",
				Ano:     2022,
			},
			texto:     "LEE, Kim. Tags <script> & outros. Science & Society, 2022.",
			htmlFinal: "LEE, Kim. Tags &lt;script&gt; &amp; outros. <strong>Science &amp; Society</strong>, 2022.",
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			texto, html := formatarABNT(c.obra, dataAcesso)
			if texto != c.texto {
				t.Errorf("texto\n got: %s\nwant: %s", texto, c.texto)
			}
			if html != c.htmlFinal {
				t.Errorf("html\n got: %s\nwant: %s", html, c.htmlFinal)
			}
		})
	}
}

func TestAutoriaABNT(t *testing.T) {
	tres := []Autor{
		{Sobrenome: "Silva", Prenome: "Ana"},
		{Sobrenome: "Santos Filho", Prenome: "José"},
		{Sobrenome: "Lima", Prenome: "Rui"},
	}
	if got, want := autoriaABNT(tres), "SILVA, Ana; SANTOS FILHO, José; LIMA, Rui"; got != want {
		t.Errorf("três autores: got %q, want %q", got, want)
	}
	if got := autoriaABNT(nil); got != "" {
		t.Errorf("sem autores: got %q, want vazio", got)
	}
}

func TestDataABNT(t *testing.T) {
	casos := map[string]time.Time{
		"5 maio 2026": time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC),
		// 02h UTC do dia 1º ainda é dia 30 em Brasília.
		"30 set. 2026": time.Date(2026, 10, 1, 2, 0, 0, 0, time.UTC),
		"1 jan. 2027":  time.Date(2027, 1, 1, 15, 0, 0, 0, time.UTC),
	}
	for want, data := range casos {
		if got := dataABNT(data); got != want {
			t.Errorf("dataABNT(%s) = %q, want %q", data, got, want)
		}
	}
}

func TestAutorDoNomeCompleto(t *testing.T) {
	casos := []struct {
		nome string
		want Autor
	}{
		{"Saleema Amershi", Autor{Sobrenome: "Amershi", Prenome: "Saleema"}},
		{"João da Silva", Autor{Sobrenome: "Silva", Prenome: "João da"}},
		{"José Santos Filho", Autor{Sobrenome: "Santos Filho", Prenome: "José"}},
		{"Carlos Lima Júnior", Autor{Sobrenome: "Lima Júnior", Prenome: "Carlos"}},
		{"Neto Filho", Autor{Sobrenome: "Filho", Prenome: "Neto"}},
		{"Madonna", Autor{Sobrenome: "Madonna"}},
		{"  ", Autor{}},
	}
	for _, c := range casos {
		if got := autorDoNomeCompleto(c.nome); got != c.want {
			t.Errorf("autorDoNomeCompleto(%q) = %+v, want %+v", c.nome, got, c.want)
		}
	}
}
