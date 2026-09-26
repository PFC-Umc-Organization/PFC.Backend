package referencia

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/projeto"
)

func requisicao(query map[string]string) events.APIGatewayProxyRequest {
	return events.APIGatewayProxyRequest{QueryStringParameters: query}
}

func TestHandleBuscar(t *testing.T) {
	servidorFalso(t, &openAlexURL, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(openAlexBusca))
	})

	resp, _ := HandleBuscar(context.Background(), requisicao(map[string]string{"q": "gestão de projetos", "pagina": "2"}))
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, resp.Body)
	}

	var resultado ResultadoBusca
	if err := json.Unmarshal([]byte(resp.Body), &resultado); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	if resultado.Total != 639 || resultado.Pagina != 2 || len(resultado.Artigos) != 2 {
		t.Errorf("resultado inesperado: %+v", resultado)
	}
	if got := resultado.Artigos[0].Autores; len(got) != 2 || got[1] != "SANTOS FILHO, José" {
		t.Errorf("autores = %v", got)
	}
}

func TestHandleBuscarValidacao(t *testing.T) {
	casos := map[string]map[string]string{
		"sem termo":             {},
		"termo curto":           {"q": "ab"},
		"só espaços":            {"q": "     "},
		"termo longo":           {"q": strings.Repeat("a", 201)},
		"página não numérica":   {"q": "gestão", "pagina": "x"},
		"página zero":           {"q": "gestão", "pagina": "0"},
		"página além do máximo": {"q": "gestão", "pagina": "51"},
	}
	for nome, query := range casos {
		resp, _ := HandleBuscar(context.Background(), requisicao(query))
		if resp.StatusCode != 400 {
			t.Errorf("%s: status = %d, want 400", nome, resp.StatusCode)
		}
	}
}

func TestHandleBuscarServicoFora(t *testing.T) {
	servidorFalso(t, &openAlexURL, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	resp, _ := HandleBuscar(context.Background(), requisicao(map[string]string{"q": "gestão"}))
	if resp.StatusCode != 502 {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
	if resp.Headers["Content-Type"] != "application/json" {
		t.Error("erro sem os headers padrão")
	}
}

func TestHandleConsultarDOI(t *testing.T) {
	servidorFalso(t, &crossrefURL, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/works/10.1145/3290605.3300233" {
			w.Write([]byte(crossrefCHI))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	anterior := agora
	agora = func() time.Time { return dataAcesso }
	t.Cleanup(func() { agora = anterior })

	resp, _ := HandleConsultarDOI(context.Background(), requisicao(map[string]string{"doi": "https://doi.org/10.1145/3290605.3300233"}))
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, resp.Body)
	}
	var ref ReferenciaFormatada
	if err := json.Unmarshal([]byte(resp.Body), &ref); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	wantABNT := "AMERSHI, Saleema; WELD, Dan; MICROSOFT RESEARCH. Guidelines for Human-AI Interaction. In: " +
		"Proceedings of the 2019 CHI Conference on Human Factors in Computing Systems. New York, NY, USA: ACM, 2019. " +
		"p. 1-13. DOI: 10.1145/3290605.3300233. Disponível em: https://doi.org/10.1145/3290605.3300233. Acesso em: 26 set. 2026."
	if ref.ABNT != wantABNT {
		t.Errorf("abnt\n got: %s\nwant: %s", ref.ABNT, wantABNT)
	}
	if !strings.Contains(ref.ABNTHTML, "<strong>Proceedings") || ref.DOI != "10.1145/3290605.3300233" || ref.Citacoes != 1234 {
		t.Errorf("resposta incompleta: %+v", ref)
	}

	if resp, _ := HandleConsultarDOI(context.Background(), requisicao(map[string]string{"doi": "10.1145/nao-existe"})); resp.StatusCode != 404 {
		t.Errorf("DOI inexistente: status = %d, want 404", resp.StatusCode)
	}
	if resp, _ := HandleConsultarDOI(context.Background(), requisicao(map[string]string{"doi": "isso não é doi"})); resp.StatusCode != 400 {
		t.Errorf("DOI inválido: status = %d, want 400", resp.StatusCode)
	}
}

func TestPermissoes(t *testing.T) {
	p := projeto.Projeto{ID: "p1", Integrantes: []string{"11111111", "22222222"}}

	casos := []struct {
		nome          string
		perfil, rgm   string
		ver, escrever bool
	}{
		{"integrante (aluno sem custom:perfil)", "", "11111111", true, true},
		{"integrante com perfil ALUNO explícito", "ALUNO", "22222222", true, true},
		{"aluno de outro grupo", "", "33333333", false, false},
		{"aluno sem e-mail no token", "", "", false, false},
		{"professor", "PROFESSOR", "alessandro.horas", true, false},
		{"coordenador", "COORDENADOR", "coordenacao", true, false},
		{"coordenador cujo usuário coincide com um RGM", "COORDENADOR", "11111111", true, false},
	}
	for _, c := range casos {
		if got := podeVer(c.perfil, c.rgm, p); got != c.ver {
			t.Errorf("%s: podeVer = %v, want %v", c.nome, got, c.ver)
		}
		if got := podeEditar(c.perfil, c.rgm, p); got != c.escrever {
			t.Errorf("%s: podeEditar = %v, want %v", c.nome, got, c.escrever)
		}
	}
}
