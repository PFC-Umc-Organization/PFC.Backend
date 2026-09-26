package referencia

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// servidorFalso sobe um httptest.Server e aponta a API indicada pra ele
// durante o teste.
func servidorFalso(t *testing.T, alvo *string, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	anterior := *alvo
	*alvo = srv.URL
	t.Cleanup(func() {
		*alvo = anterior
		srv.Close()
	})
}

// Recorte real de https://api.crossref.org/works/10.1145/3290605.3300233,
// com um autor-entidade e marcação HTML no título acrescentados pra cobrir
// esses casos.
const crossrefCHI = `{
  "status": "ok",
  "message": {
    "DOI": "10.1145/3290605.3300233",
    "type": "proceedings-article",
    "title": ["Guidelines for <i>Human-AI</i> Interaction"],
    "container-title": ["Proceedings of the 2019 CHI Conference on Human Factors in Computing Systems"],
    "publisher": "ACM",
    "publisher-location": "New York, NY, USA",
    "page": "1-13",
    "URL": "https://doi.org/10.1145/3290605.3300233",
    "is-referenced-by-count": 1234,
    "author": [
      {"given": "Saleema", "family": "Amershi"},
      {"given": "Dan", "family": "Weld"},
      {"name": "Microsoft Research"}
    ],
    "issued": {"date-parts": [[2019, 5, 2]]}
  }
}`

func TestBuscarPorDOI(t *testing.T) {
	servidorFalso(t, &crossrefURL, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/works/10.1145/3290605.3300233" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("requisição sem User-Agent — o Crossref pede identificação")
		}
		w.Write([]byte(crossrefCHI))
	})

	obra, err := buscarPorDOI(context.Background(), "10.1145/3290605.3300233")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	want := Obra{
		DOI:     "10.1145/3290605.3300233",
		Tipo:    TipoEvento,
		Titulo:  "Guidelines for Human-AI Interaction",
		Veiculo: "Proceedings of the 2019 CHI Conference on Human Factors in Computing Systems",
		Editora: "ACM",
		Local:   "New York, NY, USA",
		Paginas: "1-13",
		Ano:     2019,
		URL:     "https://doi.org/10.1145/3290605.3300233",
		Autores: []Autor{
			{Sobrenome: "Amershi", Prenome: "Saleema"},
			{Sobrenome: "Weld", Prenome: "Dan"},
			{Entidade: "Microsoft Research"},
		},
		Citacoes: 1234,
	}
	if !reflect.DeepEqual(obra, want) {
		t.Errorf("obra\n got: %+v\nwant: %+v", obra, want)
	}
}

func TestBuscarPorDOIDataNula(t *testing.T) {
	servidorFalso(t, &crossrefURL, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"message":{"DOI":"10.1/x","type":"book","title":["Livro"],"issued":{"date-parts":[[null]]}}}`))
	})

	obra, err := buscarPorDOI(context.Background(), "10.1/x")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if obra.Ano != 0 || obra.Tipo != TipoLivro {
		t.Errorf("got ano=%d tipo=%s, want ano=0 tipo=LIVRO", obra.Ano, obra.Tipo)
	}
}

func TestBuscarPorDOIErros(t *testing.T) {
	casos := []struct {
		nome   string
		status int
		corpo  string
		want   error
	}{
		{"DOI inexistente", http.StatusNotFound, "Resource not found.", errNaoEncontrado},
		{"Crossref fora do ar", http.StatusServiceUnavailable, "", errServicoExterno},
		{"resposta que não é JSON", http.StatusOK, "<html>", errServicoExterno},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			servidorFalso(t, &crossrefURL, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				w.Write([]byte(c.corpo))
			})
			_, err := buscarPorDOI(context.Background(), "10.1145/x")
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
		})
	}
}

// Recorte do formato de https://api.openalex.org/works?search=...
const openAlexBusca = `{
  "meta": {"count": 639, "page": 2, "per_page": 10},
  "results": [
    {
      "doi": "https://doi.org/10.13058/raep.2015.v16n3.283",
      "title": "Limitações no ensino de gerenciamento de projetos",
      "publication_year": 2015,
      "cited_by_count": 4,
      "authorships": [
        {"author": {"display_name": "Maria Clara Souza"}},
        {"author": {"display_name": "José Santos Filho"}}
      ],
      "primary_location": {
        "landing_page_url": "https://raep.emnuvens.com.br/raep/article/view/283",
        "source": {"display_name": "Revista de Administração, Ensino e Pesquisa"}
      }
    },
    {
      "doi": null,
      "title": "Trabalho sem DOI",
      "publication_year": null,
      "cited_by_count": 0,
      "authorships": [],
      "primary_location": {"landing_page_url": "https://repositorio.exemplo.br/123", "source": null}
    }
  ]
}`

func TestBuscarPorTema(t *testing.T) {
	servidorFalso(t, &openAlexURL, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/works" || q.Get("search") != "gestão de projetos" ||
			q.Get("page") != "2" || q.Get("per-page") != "10" || q.Get("select") != camposOpenAlex {
			t.Errorf("requisição inesperada: %s", r.URL.String())
		}
		w.Write([]byte(openAlexBusca))
	})

	total, obras, err := buscarPorTema(context.Background(), "gestão de projetos", 2)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if total != 639 || len(obras) != 2 {
		t.Fatalf("total=%d, obras=%d; want 639, 2", total, len(obras))
	}

	primeira := obras[0]
	if primeira.DOI != "10.13058/raep.2015.v16n3.283" {
		t.Errorf("DOI = %q, want sem o prefixo https://doi.org/", primeira.DOI)
	}
	wantAutores := []Autor{
		{Sobrenome: "Souza", Prenome: "Maria Clara"},
		{Sobrenome: "Santos Filho", Prenome: "José"},
	}
	if !reflect.DeepEqual(primeira.Autores, wantAutores) {
		t.Errorf("autores = %+v, want %+v", primeira.Autores, wantAutores)
	}
	if primeira.Veiculo != "Revista de Administração, Ensino e Pesquisa" || primeira.Ano != 2015 || primeira.Citacoes != 4 {
		t.Errorf("metadados inesperados: %+v", primeira)
	}

	segunda := obras[1]
	if segunda.DOI != "" || segunda.Ano != 0 || segunda.URL != "https://repositorio.exemplo.br/123" {
		t.Errorf("obra sem DOI mal convertida: %+v", segunda)
	}
}
