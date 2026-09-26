package referencia

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// Endereços das APIs como variáveis (e não constantes) pra que os testes
// apontem pra um httptest.Server.
var (
	crossrefURL = "https://api.crossref.org"
	openAlexURL = "https://api.openalex.org"

	// Timeout curto: a Lambda tem 10s no total e o front precisa de uma
	// resposta de erro antes disso se a API externa travar.
	httpClient = &http.Client{Timeout: 5 * time.Second}

	// Crossref e OpenAlex dão prioridade (o "polite pool") a quem se
	// identifica com um e-mail de contato. Opcional.
	contatoEmail = os.Getenv("REFERENCIAS_CONTATO_EMAIL")
	// Opcional: a OpenAlex funciona sem chave, mas aceita uma pra limites
	// maiores.
	openAlexAPIKey = os.Getenv("OPENALEX_API_KEY")
)

var (
	errNaoEncontrado  = errors.New("obra não encontrada")
	errServicoExterno = errors.New("serviço externo indisponível")
)

// tamanho máximo lido de uma resposta — uma busca de 10 resultados fica na
// casa das dezenas de KB.
const limiteResposta = 2 << 20

func obterJSON(ctx context.Context, endereco string, destino any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endereco, nil)
	if err != nil {
		return err
	}
	agente := "Athena-PFC/1.0"
	if contatoEmail != "" {
		agente += " (mailto:" + contatoEmail + ")"
	}
	req.Header.Set("User-Agent", agente)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", errServicoExterno, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return errNaoEncontrado
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", errServicoExterno, resp.StatusCode)
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, limiteResposta)).Decode(destino); err != nil {
		return fmt.Errorf("%w: resposta inválida: %v", errServicoExterno, err)
	}
	return nil
}

var tagHTML = regexp.MustCompile(`<[^>]*>`)

// limparTexto tira marcação que as APIs deixam passar em títulos (ex:
// "<i>Drosophila</i>", "&amp;") e normaliza os espaços.
func limparTexto(s string) string {
	s = tagHTML.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

func primeiro(valores []string) string {
	if len(valores) == 0 {
		return ""
	}
	return limparTexto(valores[0])
}
