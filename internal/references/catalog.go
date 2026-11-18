package references

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Cliente
// ---------------------------------------------------------------------------

var (
	errWorkNotFound = errors.New("work not found")
	errUnavailable  = errors.New("external service unavailable")
)

const maxResponseSize = 2 << 20

// Configuração
type CatalogConfig struct {
	CrossrefURL    string
	OpenAlexURL    string
	ContactEmail   string
	OpenAlexAPIKey string
}

// Cliente
type Catalog struct {
	cfg    CatalogConfig
	client *http.Client
}

// Construção
func NewCatalog(cfg CatalogConfig) *Catalog {

	return &Catalog{cfg: cfg, client: &http.Client{Timeout: 5 * time.Second}}
}

// Requisição
func (c *Catalog) getJSON(ctx context.Context, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	agent := "RafaManager-PFC/1.0"
	if c.cfg.ContactEmail != "" {
		agent += " (mailto:" + c.cfg.ContactEmail + ")"
	}
	req.Header.Set("User-Agent", agent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", errUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errWorkNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", errUnavailable, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(dst); err != nil {
		return fmt.Errorf("%w: invalid response: %w", errUnavailable, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Crossref
// ---------------------------------------------------------------------------

// Resposta
type crossrefResponse struct {
	Message crossrefWork `json:"message"`
}

// Resposta
type crossrefWork struct {
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
		DateParts [][]*int `json:"date-parts"`
	} `json:"issued"`
}

// Consulta
func (c *Catalog) WorkByDOI(ctx context.Context, doi string) (Work, error) {
	parts := strings.Split(doi, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	endpoint := c.cfg.CrossrefURL + "/works/" + strings.Join(parts, "/")
	if c.cfg.ContactEmail != "" {
		endpoint += "?mailto=" + url.QueryEscape(c.cfg.ContactEmail)
	}
	var resp crossrefResponse
	if err := c.getJSON(ctx, endpoint, &resp); err != nil {
		return Work{}, err
	}
	return resp.Message.work(), nil
}

// Mapeamento
func (cw crossrefWork) work() Work {
	w := Work{
		DOI:       cw.DOI,
		Type:      crossrefType(cw.Type),
		Title:     first(cw.Title),
		Subtitle:  first(cw.Subtitle),
		Venue:     first(cw.ContainerTitle),
		Publisher: clean(cw.Publisher),
		Place:     clean(cw.PublisherLocation),
		Volume:    cw.Volume,
		Issue:     cw.Issue,
		Pages:     cw.Page,
		URL:       cw.URL,
		Citations: cw.ReferencedBy,
	}
	if d := cw.Issued.DateParts; len(d) > 0 && len(d[0]) > 0 && d[0][0] != nil {
		w.Year = *d[0][0]
	}
	for _, a := range cw.Author {
		switch {
		case a.Family != "":
			w.Authors = append(w.Authors, Author{Surname: clean(a.Family), GivenName: clean(a.Given)})
		case a.Name != "":
			w.Authors = append(w.Authors, Author{Organization: clean(a.Name)})
		}
	}
	return w
}

// Mapeamento
func crossrefType(t string) WorkType {
	switch t {
	case "journal-article":
		return WorkArticle
	case "proceedings-article":
		return WorkConferencePaper
	case "book-chapter", "book-section", "book-part":
		return WorkChapter
	case "book", "monograph", "edited-book", "reference-book":
		return WorkBook
	}
	return WorkOther
}

// ---------------------------------------------------------------------------
// OpenAlex
// ---------------------------------------------------------------------------

const openAlexFields = "doi,title,publication_year,cited_by_count,authorships,primary_location"

// Resposta
type openAlexResponse struct {
	Meta struct {
		Count int `json:"count"`
	} `json:"meta"`
	Results []openAlexWork `json:"results"`
}

// Resposta
type openAlexWork struct {
	DOI         *string `json:"doi"`
	Title       *string `json:"title"`
	Year        *int    `json:"publication_year"`
	CitedBy     int     `json:"cited_by_count"`
	Authorships []struct {
		Author struct {
			DisplayName string `json:"display_name"`
		} `json:"author"`
	} `json:"authorships"`
	PrimaryLocation *struct {
		LandingPageURL *string `json:"landing_page_url"`
		Source         *struct {
			DisplayName string `json:"display_name"`
		} `json:"source"`
	} `json:"primary_location"`
}

// Busca
func (c *Catalog) Search(ctx context.Context, term string, page int) (int, []Work, error) {
	params := url.Values{}
	params.Set("search", term)
	params.Set("page", strconv.Itoa(page))
	params.Set("per-page", strconv.Itoa(pageSize))
	params.Set("select", openAlexFields)
	if c.cfg.ContactEmail != "" {
		params.Set("mailto", c.cfg.ContactEmail)
	}
	if c.cfg.OpenAlexAPIKey != "" {
		params.Set("api_key", c.cfg.OpenAlexAPIKey)
	}
	var resp openAlexResponse
	if err := c.getJSON(ctx, c.cfg.OpenAlexURL+"/works?"+params.Encode(), &resp); err != nil {
		return 0, nil, err
	}
	works := make([]Work, 0, len(resp.Results))
	for _, r := range resp.Results {
		works = append(works, r.work())
	}
	return resp.Meta.Count, works, nil
}

// Mapeamento
func (ow openAlexWork) work() Work {
	w := Work{Type: WorkOther, Citations: ow.CitedBy}
	if ow.DOI != nil {

		w.DOI, _ = normalizeDOI(*ow.DOI)
		w.URL = *ow.DOI
	}
	if ow.Title != nil {
		w.Title = clean(*ow.Title)
	}
	if ow.Year != nil {
		w.Year = *ow.Year
	}
	if loc := ow.PrimaryLocation; loc != nil {
		if loc.Source != nil {
			w.Venue = clean(loc.Source.DisplayName)
		}
		if w.URL == "" && loc.LandingPageURL != nil {
			w.URL = *loc.LandingPageURL
		}
	}
	for _, a := range ow.Authorships {
		if name := clean(a.Author.DisplayName); name != "" {
			w.Authors = append(w.Authors, authorFromFullName(name))
		}
	}
	return w
}

// ---------------------------------------------------------------------------
// Autores
// ---------------------------------------------------------------------------

var kinshipSuffixes = map[string]bool{
	"filho": true, "filha": true, "neto": true, "neta": true,
	"sobrinho": true, "sobrinha": true, "junior": true, "júnior": true, "jr.": true,
}

// Mapeamento
func authorFromFullName(name string) Author {
	parts := strings.Fields(name)
	switch len(parts) {
	case 0:
		return Author{}
	case 1:
		return Author{Surname: parts[0]}
	}
	end := len(parts) - 1
	surname := parts[end]
	if kinshipSuffixes[strings.ToLower(surname)] && end >= 2 {
		end--
		surname = parts[end] + " " + surname
	}
	return Author{Surname: surname, GivenName: strings.Join(parts[:end], " ")}
}

// ---------------------------------------------------------------------------
// Texto
// ---------------------------------------------------------------------------

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// Limpeza
func clean(s string) string {
	s = htmlTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

// Seleção
func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return clean(values[0])
}
