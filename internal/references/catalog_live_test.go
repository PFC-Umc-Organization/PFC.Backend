package references

import (
	"context"
	"os"
	"strings"
	"testing"
)

// These hit the real APIs and are off by default:
//
//	REFERENCIAS_INTEGRACAO=1 go test ./internal/references -run Live -v
func liveCatalog(t *testing.T) *Catalog {
	t.Helper()
	if os.Getenv("REFERENCIAS_INTEGRACAO") == "" {
		t.Skip("set REFERENCIAS_INTEGRACAO=1 to run against the real APIs")
	}
	return NewCatalog(CatalogConfig{CrossrefURL: "https://api.crossref.org", OpenAlexURL: "https://api.openalex.org"})
}

func TestLiveCrossref(t *testing.T) {
	c := liveCatalog(t)
	w, err := c.WorkByDOI(context.Background(), "10.1145/3290605.3300233")
	if err != nil {
		t.Fatalf("Crossref: %v", err)
	}
	text, _ := formatABNT(w, accessDate)
	t.Logf("ABNT: %s", text)
	if w.Type != WorkConferencePaper || w.Year != 2019 || !strings.HasPrefix(text, "AMERSHI, Saleema et al.") {
		t.Errorf("metadata = %+v", w)
	}
	if _, err := c.WorkByDOI(context.Background(), "10.1145/doi-que-nao-existe-athena"); err != errWorkNotFound {
		t.Errorf("unknown DOI: err = %v", err)
	}
}

func TestLiveOpenAlex(t *testing.T) {
	c := liveCatalog(t)
	total, works, err := c.Search(context.Background(), "gerenciamento de projetos acadêmicos", 1)
	if err != nil {
		t.Fatalf("OpenAlex: %v", err)
	}
	for _, w := range works {
		t.Logf("- %d | %s | %s | %d authors", w.Year, w.Title, w.DOI, len(w.Authors))
	}
	if total == 0 || len(works) == 0 || len(works) > pageSize {
		t.Errorf("total = %d, works = %d", total, len(works))
	}
}
