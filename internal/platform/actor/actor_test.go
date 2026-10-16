package actor

import "testing"

func TestHasRole(t *testing.T) {
	a := Actor{Role: RoleAdvisor}
	if !a.HasRole(RoleAdmin, RoleAdvisor) || a.HasRole(RoleAdmin) || a.HasRole() {
		t.Fatal("HasRole mismatch")
	}
	if (Actor{}).HasRole(RoleStudent) {
		t.Fatal("an actor without role must not match ALUNO")
	}
}

func TestRGM(t *testing.T) {
	for email, want := range map[string]string{
		"12345678901@alunos.umc.br": "12345678901",
		"prof@umc.br":               "prof",
		"semarroba":                 "",
		"":                          "",
	} {
		if got := (Actor{Email: email}).RGM(); got != want {
			t.Errorf("RGM(%q) = %q, want %q", email, got, want)
		}
	}
}
