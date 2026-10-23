package storage

import (
	"strings"
	"testing"
)

func TestSafeName(t *testing.T) {
	long := strings.Repeat("a", 200) + ".pdf"
	for in, want := range map[string]string{
		" relatório/final.pdf ": "relatório-final.pdf",
		"a\\b\x00.zip":          "a-b-.zip",
		long:                    strings.Repeat("a", maxNameLen-4) + ".pdf",
	} {
		if got := SafeName(in); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestContentType(t *testing.T) {
	if ct, ok := ContentType("Relatório.PDF"); !ok || ct != "application/pdf" {
		t.Errorf("pdf = %q %v", ct, ok)
	}
	if _, ok := ContentType("script.exe"); ok {
		t.Error("exe must be refused")
	}
}
