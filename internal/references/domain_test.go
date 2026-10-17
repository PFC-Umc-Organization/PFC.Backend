package references

import "testing"

func TestNormalizeDOI(t *testing.T) {
	valid := map[string]string{
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
	for in, want := range valid {
		if got, ok := normalizeDOI(in); !ok || got != want {
			t.Errorf("normalizeDOI(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "abc", "10.1145", "10.1145/", "11.1145/123", "10./123", "https://exemplo.com/artigo", "10.1145/com espaço"} {
		if got, ok := normalizeDOI(in); ok {
			t.Errorf("normalizeDOI(%q) = %q, want invalid", in, got)
		}
	}
}

func TestReferenceID(t *testing.T) {
	a := referenceID("10.1145/ABC.123")
	if a != referenceID("10.1145/abc.123") {
		t.Error("DOIs are case-insensitive: ids must match")
	}
	if len(a) != 16 {
		t.Errorf("id has %d characters, want 16", len(a))
	}
	if referenceID("10.1145/other") == a {
		t.Error("different DOIs produced the same id")
	}
}
