package referencia

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Testes contra as APIs reais. Ficam desligados por padrão (dependem de
// rede e de serviço de terceiros) — rode com:
//
//	REFERENCIAS_INTEGRACAO=1 go test ./internal/referencia -run Integracao -v
func pularSemIntegracao(t *testing.T) {
	t.Helper()
	if os.Getenv("REFERENCIAS_INTEGRACAO") == "" {
		t.Skip("defina REFERENCIAS_INTEGRACAO=1 pra rodar contra as APIs reais")
	}
}

func TestIntegracaoCrossref(t *testing.T) {
	pularSemIntegracao(t)

	obra, err := buscarPorDOI(context.Background(), "10.1145/3290605.3300233")
	if err != nil {
		t.Fatalf("Crossref: %v", err)
	}
	texto, _ := formatarABNT(obra, dataAcesso)
	t.Logf("ABNT: %s", texto)

	if obra.Tipo != TipoEvento || obra.Ano != 2019 || !strings.HasPrefix(texto, "AMERSHI, Saleema et al.") {
		t.Errorf("metadados inesperados: %+v", obra)
	}

	if _, err := buscarPorDOI(context.Background(), "10.1145/doi-que-nao-existe-athena"); err != errNaoEncontrado {
		t.Errorf("DOI inexistente: err = %v, want errNaoEncontrado", err)
	}
}

func TestIntegracaoOpenAlex(t *testing.T) {
	pularSemIntegracao(t)

	total, obras, err := buscarPorTema(context.Background(), "gerenciamento de projetos acadêmicos", 1)
	if err != nil {
		t.Fatalf("OpenAlex: %v", err)
	}
	t.Logf("total: %d", total)
	for _, o := range obras {
		t.Logf("- %d | %s | %s | %d autores", o.Ano, o.Titulo, o.DOI, len(o.Autores))
	}
	if total == 0 || len(obras) == 0 || len(obras) > porPagina {
		t.Errorf("total=%d, obras=%d", total, len(obras))
	}
}
