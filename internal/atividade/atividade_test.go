package atividade

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func reqComPerfil(perfil string) events.APIGatewayProxyRequest {
	return events.APIGatewayProxyRequest{
		RequestContext: events.APIGatewayProxyRequestContext{
			Authorizer: map[string]interface{}{
				"claims": map[string]interface{}{"custom:perfil": perfil},
			},
		},
	}
}

func TestEscritaSoParaEquipeAcademica(t *testing.T) {
	handlers := map[string]func(context.Context, events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error){
		"criar":          HandleCriar,
		"atualizar":      HandleAtualizar,
		"deletar":        HandleDeletar,
		"adicionarCampo": HandleAdicionarCampo,
		"removerCampo":   HandleRemoverCampo,
		"entregasAtiv":   HandleListarEntregasDaAtividade,
	}
	for nome, h := range handlers {
		resp, _ := h(context.Background(), reqComPerfil("ALUNO"))
		if resp.StatusCode != 403 {
			t.Errorf("%s: aluno recebeu %d, esperado 403", nome, resp.StatusCode)
		}
	}
}

func TestNormalizarDados(t *testing.T) {
	if _, msg := normalizarDados("  Tema  ", "desc", "2026-10-15T23:59"); msg != "" {
		t.Errorf("dados válidos recusados: %s", msg)
	}
	casos := map[string][3]string{
		"sem título":     {"  ", "d", "2026-10-15T23:59"},
		"prazo inválido": {"Tema", "d", "15/10/2026"},
		"título longo":   {strings.Repeat("a", 151), "d", "2026-10-15T23:59"},
	}
	for nome, c := range casos {
		if _, msg := normalizarDados(c[0], c[1], c[2]); msg == "" {
			t.Errorf("%s: esperava erro", nome)
		}
	}
}

func TestNormalizarCampo(t *testing.T) {
	if _, msg := normalizarCampo(NovoCampo{Rotulo: " Repo ", Tipo: CampoLink}); msg != "" {
		t.Errorf("campo válido recusado: %s", msg)
	}
	if _, msg := normalizarCampo(NovoCampo{Rotulo: "", Tipo: CampoLink}); msg == "" {
		t.Error("rótulo vazio deveria falhar")
	}
	if _, msg := normalizarCampo(NovoCampo{Rotulo: "x", Tipo: "XYZ"}); msg == "" {
		t.Error("tipo desconhecido deveria falhar")
	}
}

func TestValidarRespostas(t *testing.T) {
	campos := []Campo{
		{ID: "c1", Rotulo: "Arquivo", Tipo: CampoArquivo, Obrigatorio: true},
		{ID: "c2", Rotulo: "Repo", Tipo: CampoLink, Obrigatorio: false},
		{ID: "c3", Rotulo: "Resumo", Tipo: CampoTexto, Obrigatorio: false},
	}

	limpas, msg := validarRespostas(campos, map[string]string{"c1": " tema.pdf ", "c2": "https://github.com/x/y", "c3": "  "})
	if msg != "" {
		t.Fatalf("respostas válidas recusadas: %s", msg)
	}
	if limpas["c1"] != "tema.pdf" || len(limpas) != 2 {
		t.Errorf("respostas limpas inesperadas: %v", limpas)
	}

	invalidos := map[string]map[string]string{
		"obrigatório faltando": {"c2": "https://x.com"},
		"campo desconhecido":   {"c1": "a.pdf", "zzz": "x"},
		"link sem http":        {"c1": "a.pdf", "c2": "github.com/x"},
		"texto longo demais":   {"c1": "a.pdf", "c3": strings.Repeat("a", 301)},
	}
	for nome, r := range invalidos {
		if _, msg := validarRespostas(campos, r); msg == "" {
			t.Errorf("%s: esperava erro", nome)
		}
	}
}
