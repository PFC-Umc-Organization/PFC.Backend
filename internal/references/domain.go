// Package references owns a project's bibliography: search by topic
// (OpenAlex), lookup by DOI (Crossref) and ABNT NBR 6023 formatting.
package references

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// WorkType picks the ABNT layout.
type WorkType int

const (
	WorkOther WorkType = iota
	WorkArticle
	WorkConferencePaper
	WorkChapter
	WorkBook
)

// Organization is set when the author is an institution, which has no
// surname or given name.
type Author struct {
	Surname      string
	GivenName    string
	Organization string
}

// Work is the metadata of a publication, whatever API it came from.
// Venue is the journal (article), the proceedings (conference paper) or the
// book that contains the chapter.
type Work struct {
	DOI       string
	Type      WorkType
	Title     string
	Subtitle  string
	Authors   []Author
	Venue     string
	Publisher string
	Place     string
	Volume    string
	Issue     string
	Pages     string
	Year      int
	URL       string
	Citations int
}

type FoundWork struct {
	DOI       string   `json:"doi,omitempty"`
	Title     string   `json:"titulo"`
	Authors   []string `json:"autores"`
	Year      int      `json:"ano,omitempty"`
	Venue     string   `json:"veiculo,omitempty"`
	Citations int      `json:"citacoes"`
	URL       string   `json:"url,omitempty"`
}

type SearchResult struct {
	Total int         `json:"total"`
	Page  int         `json:"pagina"`
	Works []FoundWork `json:"artigos"`
}

// FormattedWork previews a reference before it joins the project's list.
type FormattedWork struct {
	FoundWork
	ABNT     string `json:"abnt"`
	ABNTHTML string `json:"abntHtml"`
}

type Reference struct {
	ID        string `json:"id"`
	ProjectID string `json:"projetoId"`
	DOI       string `json:"doi"`
	Title     string `json:"titulo"`
	Year      int    `json:"ano,omitempty"`
	ABNT      string `json:"abnt"`
	ABNTHTML  string `json:"abntHtml"`
	AddedBy   string `json:"adicionadaPor"`
	AddedAt   string `json:"adicionadaEm"`
}

// ReferenceInput carries only the DOI: the metadata is fetched again from
// Crossref so nobody stores a made-up reference.
type ReferenceInput struct {
	DOI string `json:"doi"`
}

func found(w Work) FoundWork {
	authors := make([]string, 0, len(w.Authors))
	for _, a := range w.Authors {
		authors = append(authors, a.abnt())
	}
	return FoundWork{
		DOI:       w.DOI,
		Title:     fullTitle(w),
		Authors:   authors,
		Year:      w.Year,
		Venue:     w.Venue,
		Citations: w.Citations,
		URL:       w.URL,
	}
}

var doiPrefixes = []string{
	"https://doi.org/",
	"http://doi.org/",
	"https://dx.doi.org/",
	"http://dx.doi.org/",
	"doi.org/",
	"doi:",
}

// normalizeDOI accepts a DOI the way students paste it ("DOI: 10.1145/...",
// "https://doi.org/10.1145/...") and returns the bare identifier.
func normalizeDOI(raw string) (string, bool) {
	doi := strings.TrimSpace(raw)
	lower := strings.ToLower(doi)
	for _, p := range doiPrefixes {
		if strings.HasPrefix(lower, p) {
			doi = strings.TrimSpace(doi[len(p):])
			break
		}
	}
	registrant, suffix, ok := strings.Cut(doi, "/")
	if !ok || !strings.HasPrefix(registrant, "10.") || len(registrant) < 4 || suffix == "" {
		return "", false
	}
	if strings.ContainsAny(doi, " \t\r\n") {
		return "", false
	}
	return doi, true
}

// referenceID derives a stable id from the DOI: a DOI has "/" and cannot be
// a path segment, and a deterministic id lets DynamoDB reject the same work
// twice in a project. DOIs are case-insensitive.
func referenceID(doi string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(doi)))
	return hex.EncodeToString(sum[:8])
}

const (
	minSearchLen = 3
	maxSearchLen = 200
	pageSize     = 10
	// OpenAlex pages by number only up to 10,000 results.
	maxPage = 50
)

const (
	msgSearchLength      = "informe de 3 a 200 caracteres para buscar"
	msgInvalidPage       = "página inválida"
	msgSearchUnavailable = "o serviço de busca de artigos está indisponível, tente novamente em instantes"
	msgInvalidDOI        = "DOI inválido — use o formato 10.xxxx/xxxxx"
	msgDOINotFound       = "DOI não encontrado"
	msgDOIUnavailable    = "o serviço de consulta de DOI está indisponível, tente novamente em instantes"
	msgMembersAndStaff   = "acesso restrito aos integrantes, ao orientador do projeto e à coordenação"
	msgMembersOnly       = "apenas integrantes do projeto podem alterar as referências"
	msgProjectNotFound   = "projeto não encontrado"
	msgDuplicate         = "essa referência já está na lista do projeto"
	msgReferenceNotFound = "referência não encontrada"
)
