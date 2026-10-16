package integracao

import (
	"net"
	"net/http"
	"net/url"
)

const knownDOI = "10.1145/3290605.3300233"

const crossrefWork = `{
  "status": "ok",
  "message": {
    "DOI": "10.1145/3290605.3300233",
    "type": "proceedings-article",
    "title": ["Guidelines for Human-AI Interaction"],
    "container-title": ["Proceedings of the 2019 CHI Conference on Human Factors in Computing Systems"],
    "publisher": "ACM",
    "publisher-location": "New York, NY, USA",
    "page": "1-13",
    "URL": "https://doi.org/10.1145/3290605.3300233",
    "is-referenced-by-count": 1234,
    "author": [{"given": "Saleema", "family": "Amershi"}, {"given": "Dan", "family": "Weld"}],
    "issued": {"date-parts": [[2019, 5, 2]]}
  }
}`

const openAlexSearch = `{
  "meta": {"count": 1, "page": 1, "per_page": 10},
  "results": [{
    "doi": "https://doi.org/10.1145/3290605.3300233",
    "title": "Guidelines for Human-AI Interaction",
    "publication_year": 2019,
    "cited_by_count": 1234,
    "authorships": [{"author": {"display_name": "Saleema Amershi"}}],
    "primary_location": {"landing_page_url": "https://doi.org/10.1145/3290605.3300233", "source": {"display_name": "CHI 2019"}}
  }]
}`

func serveExternalAPIs(crossrefURL, openAlexURL string) error {
	crossref := http.NewServeMux()
	crossref.HandleFunc("/works/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/works/"+knownDOI {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(crossrefWork))
	})
	openAlex := http.NewServeMux()
	openAlex.HandleFunc("/works", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(openAlexSearch))
	})
	if err := listen(crossrefURL, crossref); err != nil {
		return err
	}
	return listen(openAlexURL, openAlex)
}

func listen(endpoint string, h http.Handler) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", u.Host)
	if err != nil {
		return err
	}
	go http.Serve(ln, h)
	return nil
}
