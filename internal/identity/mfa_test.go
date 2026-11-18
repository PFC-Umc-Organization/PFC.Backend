package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
)

// ---------------------------------------------------------------------------
// MFA
// ---------------------------------------------------------------------------

// Simulação
type mfaDirectory struct {
	*fakeDirectory
	err error
	set map[string]bool
}

// Preferência
func (m *mfaDirectory) SetUserMFA(_ context.Context, username string, enabled bool) error {
	if m.err != nil {
		return m.err
	}
	m.set[username] = enabled
	return nil
}

// Alternância
func TestSetMFA(t *testing.T) {
	yes, no := true, false
	d := &mfaDirectory{fakeDirectory: &fakeDirectory{users: []User{
		{ID: "11111111-1111-4111-8111-111111111111", Name: "Prof", Role: actor.RoleAdvisor, Username: "prof@umc.br"},
		{ID: "22222222-2222-4222-8222-222222222222", Name: "Aluno", Role: actor.RoleStudent, RGM: "12345678901"},
	}}, set: map[string]bool{}}
	svc := newTestService(t, d.fakeDirectory)
	svc.directory = d
	ctx := context.Background()

	if _, err := svc.SetMFA(ctx, "11111111-1111-4111-8111-111111111111", MFAInput{}); err == nil {
		t.Error("missing field must fail")
	}
	if on, err := svc.SetMFA(ctx, "11111111-1111-4111-8111-111111111111", MFAInput{Enabled: &yes}); err != nil || !on || !d.set["prof@umc.br"] {
		t.Fatalf("enable: %v, %v", on, err)
	}
	if !profilesOf(svc).mfa["11111111-1111-4111-8111-111111111111"] {
		t.Error("flag must be saved")
	}
	plain, err := svc.ListUsers(ctx, actor.Actor{Role: actor.RoleAdmin}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range plain {
		if u.MFAEnabled {
			t.Errorf("%s: mfa must not load without incluirMfa", u.Name)
		}
	}
	users, err := svc.ListUsers(ctx, actor.Actor{Role: actor.RoleAdmin}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if want := u.Name == "Prof"; u.MFAEnabled != want {
			t.Errorf("%s: mfa = %v", u.Name, u.MFAEnabled)
		}
	}
	if _, err := svc.SetMFA(ctx, "11111111-1111-4111-8111-111111111111", MFAInput{Enabled: &no}); err != nil || d.set["prof@umc.br"] {
		t.Fatalf("disable: %v", err)
	}

	d.err = errInvalidInput
	_, err = svc.SetMFA(ctx, "11111111-1111-4111-8111-111111111111", MFAInput{Enabled: &yes})
	if e, ok := apperr.As(err); !ok || e.Kind != apperr.KindInvalid || e.Message != msgMFANotLinked {
		t.Errorf("not linked: %v", err)
	}
	d.err = errors.New("boom")
	if _, err := svc.SetMFA(ctx, "11111111-1111-4111-8111-111111111111", MFAInput{Enabled: &yes}); err == nil {
		t.Error("directory failure must fail")
	}
}
