package references

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func fakeCatalog(t *testing.T, crossref, openAlex http.HandlerFunc) *Catalog {
	t.Helper()
	cfg := CatalogConfig{}
	if crossref != nil {
		srv := httptest.NewServer(crossref)
		t.Cleanup(srv.Close)
		cfg.CrossrefURL = srv.URL
	}
	if openAlex != nil {
		srv := httptest.NewServer(openAlex)
		t.Cleanup(srv.Close)
		cfg.OpenAlexURL = srv.URL
	}
	return NewCatalog(cfg)
}

func serve(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(body)) }
}

// Excerpt of https://api.crossref.org/works/10.1145/3290605.3300233, plus an
// organization author and HTML in the title.
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

func TestWorkByDOI(t *testing.T) {
	c := fakeCatalog(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/works/10.1145/3290605.3300233" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("Crossref asks clients to identify themselves")
		}
		w.Write([]byte(crossrefCHI))
	}, nil)

	got, err := c.WorkByDOI(context.Background(), "10.1145/3290605.3300233")
	if err != nil {
		t.Fatal(err)
	}
	want := Work{
		DOI:       "10.1145/3290605.3300233",
		Type:      WorkConferencePaper,
		Title:     "Guidelines for Human-AI Interaction",
		Venue:     "Proceedings of the 2019 CHI Conference on Human Factors in Computing Systems",
		Publisher: "ACM",
		Place:     "New York, NY, USA",
		Pages:     "1-13",
		Year:      2019,
		URL:       "https://doi.org/10.1145/3290605.3300233",
		Authors: []Author{
			{Surname: "Amershi", GivenName: "Saleema"},
			{Surname: "Weld", GivenName: "Dan"},
			{Organization: "Microsoft Research"},
		},
		Citations: 1234,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("work\n got: %+v\nwant: %+v", got, want)
	}
}

func TestWorkByDOINullDate(t *testing.T) {
	c := fakeCatalog(t, serve(`{"message":{"DOI":"10.1/x","type":"book","title":["Livro"],"issued":{"date-parts":[[null]]}}}`), nil)
	w, err := c.WorkByDOI(context.Background(), "10.1/x")
	if err != nil || w.Year != 0 || w.Type != WorkBook {
		t.Errorf("work = %+v, err = %v", w, err)
	}
}

func TestWorkByDOIErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"unknown DOI", http.StatusNotFound, "Resource not found.", errWorkNotFound},
		{"Crossref down", http.StatusServiceUnavailable, "", errUnavailable},
		{"not JSON", http.StatusOK, "<html>", errUnavailable},
	}
	for _, tc := range cases {
		c := fakeCatalog(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte(tc.body))
		}, nil)
		if _, err := c.WorkByDOI(context.Background(), "10.1145/x"); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
}

// Shape of https://api.openalex.org/works?search=...
const openAlexSearch = `{
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

func TestSearch(t *testing.T) {
	c := fakeCatalog(t, nil, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/works" || q.Get("search") != "gestão de projetos" ||
			q.Get("page") != "2" || q.Get("per-page") != "10" || q.Get("select") != openAlexFields {
			t.Errorf("unexpected request: %s", r.URL.String())
		}
		w.Write([]byte(openAlexSearch))
	})

	total, works, err := c.Search(context.Background(), "gestão de projetos", 2)
	if err != nil || total != 639 || len(works) != 2 {
		t.Fatalf("total = %d, works = %d, err = %v", total, len(works), err)
	}
	first := works[0]
	if first.DOI != "10.13058/raep.2015.v16n3.283" {
		t.Errorf("DOI = %q, want it without https://doi.org/", first.DOI)
	}
	wantAuthors := []Author{{Surname: "Souza", GivenName: "Maria Clara"}, {Surname: "Santos Filho", GivenName: "José"}}
	if !reflect.DeepEqual(first.Authors, wantAuthors) {
		t.Errorf("authors = %+v", first.Authors)
	}
	if first.Venue != "Revista de Administração, Ensino e Pesquisa" || first.Year != 2015 || first.Citations != 4 {
		t.Errorf("metadata = %+v", first)
	}
	if second := works[1]; second.DOI != "" || second.Year != 0 || second.URL != "https://repositorio.exemplo.br/123" {
		t.Errorf("work without DOI = %+v", second)
	}
}
