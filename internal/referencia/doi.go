package referencia

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

var prefixosDOI = []string{
	"https://doi.org/",
	"http://doi.org/",
	"https://dx.doi.org/",
	"http://dx.doi.org/",
	"doi.org/",
	"doi:",
}

// normalizarDOI aceita o DOI do jeito que o aluno costuma colar ("DOI:
// 10.1145/...", "https://doi.org/10.1145/...") e devolve só o
// identificador. ok=false se não tiver o formato 10.<registrante>/<sufixo>.
func normalizarDOI(bruto string) (doi string, ok bool) {
	doi = strings.TrimSpace(bruto)
	minusculo := strings.ToLower(doi)
	for _, p := range prefixosDOI {
		if strings.HasPrefix(minusculo, p) {
			doi = strings.TrimSpace(doi[len(p):])
			break
		}
	}

	registrante, sufixo, temBarra := strings.Cut(doi, "/")
	if !temBarra || !strings.HasPrefix(registrante, "10.") || len(registrante) < 4 || sufixo == "" {
		return "", false
	}
	if strings.ContainsAny(doi, " \t\r\n") {
		return "", false
	}
	return doi, true
}

// idDaReferencia gera um id estável a partir do DOI. DOI tem "/" e não
// cabe num segmento de path (DELETE /projetos/:id/referencias/:refId), e
// ser determinístico faz o próprio DynamoDB barrar a mesma obra duas vezes
// no projeto. DOI não diferencia maiúsculas, por isso o ToLower.
func idDaReferencia(doi string) string {
	soma := sha256.Sum256([]byte(strings.ToLower(doi)))
	return hex.EncodeToString(soma[:8])
}
