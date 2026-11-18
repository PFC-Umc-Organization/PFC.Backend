package references

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// ---------------------------------------------------------------------------
// Obras
// ---------------------------------------------------------------------------

// Enumeração
type WorkType int

const (
	WorkOther WorkType = iota
	WorkArticle
	WorkConferencePaper
	WorkChapter
	WorkBook
)

// Modelo
type Author struct {
	Surname      string
	GivenName    string
	Organization string
}

// Modelo
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

// Resposta
type FoundWork struct {
	DOI       string   `json:"doi,omitempty"`
	Title     string   `json:"titulo"`
	Authors   []string `json:"autores"`
	Year      int      `json:"ano,omitempty"`
	Venue     string   `json:"veiculo,omitempty"`
	Citations int      `json:"citacoes"`
	URL       string   `json:"url,omitempty"`
}

// Resposta
type SearchResult struct {
	Total int         `json:"total"`
	Page  int         `json:"pagina"`
	Works []FoundWork `json:"artigos"`
}

// Resposta
type FormattedWork struct {
	FoundWork
	ABNT     string `json:"abnt"`
	ABNTHTML string `json:"abntHtml"`
}

// ---------------------------------------------------------------------------
// Referências
// ---------------------------------------------------------------------------

// Modelo
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

// Entrada
type ReferenceInput struct {
	DOI string `json:"doi"`
}

// Mapeamento
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

// ---------------------------------------------------------------------------
// DOI
// ---------------------------------------------------------------------------

var doiPrefixes = []string{
	"https://doi.org/",
	"http://doi.org/",
	"https://dx.doi.org/",
	"http://dx.doi.org/",
	"doi.org/",
	"doi:",
}

// Normalização
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

// Identificação
func referenceID(doi string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(doi)))
	return hex.EncodeToString(sum[:8])
}

// ---------------------------------------------------------------------------
// Mensagens
// ---------------------------------------------------------------------------

const (
	minSearchLen = 3
	maxSearchLen = 200
	pageSize     = 10

	maxPage = 50
)

const (
	msgSearchLength      = "Informe de 3 a 200 caracteres para a busca."
	msgInvalidPage       = "Página inválida."
	msgSearchUnavailable = "O serviço de busca de artigos está indisponível. Tente novamente em instantes."
	msgInvalidDOI        = "DOI inválido. Use o formato 10.xxxx/xxxxx."
	msgDOINotFound       = "DOI não encontrado na base da Crossref."
	msgDOIUnavailable    = "O serviço de consulta de DOI está indisponível. Tente novamente em instantes."
	msgMembersAndStaff   = "Acesso restrito aos integrantes, ao orientador do projeto e à coordenação."
	msgMembersOnly       = "Apenas integrantes do projeto podem alterar as referências."
	msgProjectNotFound   = "Projeto não encontrado."
	msgDuplicate         = "Esta referência já está na lista do projeto."
	msgReferenceNotFound = "Referência não encontrada."
)
