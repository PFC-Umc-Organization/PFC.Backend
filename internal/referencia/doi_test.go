package referencia

import "testing"

func TestNormalizarDOI(t *testing.T) {
	validos := map[string]string{
		"10.1145/3290605.3300233":                    "10.1145/3290605.3300233",
		"  10.1145/3290605.3300233  ":                "10.1145/3290605.3300233",
		"https://doi.org/10.1145/3290605.3300233":    "10.1145/3290605.3300233",
		"HTTPS://DOI.ORG/10.1145/3290605.3300233":    "10.1145/3290605.3300233",
		"http://dx.doi.org/10.1145/3290605.3300233":  "10.1145/3290605.3300233",
		"doi:10.1145/3290605.3300233":                "10.1145/3290605.3300233",
		"DOI: 10.1145/3290605.3300233":               "10.1145/3290605.3300233",
		"10.1002/(SICI)1097-4571(199806)49:8<693::A": "10.1002/(SICI)1097-4571(199806)49:8<693::A",
		"10.13058/raep.2015.v16n3.283":               "10.13058/raep.2015.v16n3.283",
	}
	for entrada, want := range validos {
		got, ok := normalizarDOI(entrada)
		if !ok || got != want {
			t.Errorf("normalizarDOI(%q) = %q, %v; want %q, true", entrada, got, ok, want)
		}
	}

	invalidos := []string{
		"",
		"abc",
		"10.1145",
		"10.1145/",
		"11.1145/123",
		"10./123",
		"https://exemplo.com/artigo",
		"10.1145/com espaço",
	}
	for _, entrada := range invalidos {
		if got, ok := normalizarDOI(entrada); ok {
			t.Errorf("normalizarDOI(%q) = %q, true; want inválido", entrada, got)
		}
	}
}

func TestIdDaReferencia(t *testing.T) {
	a := idDaReferencia("10.1145/ABC.123")
	b := idDaReferencia("10.1145/abc.123")
	if a != b {
		t.Errorf("DOI não diferencia maiúsculas, ids deveriam ser iguais: %s != %s", a, b)
	}
	if len(a) != 16 {
		t.Errorf("id com %d caracteres, want 16", len(a))
	}
	if idDaReferencia("10.1145/outro") == a {
		t.Error("DOIs diferentes geraram o mesmo id")
	}
}
