package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

type fakeDirectory struct {
	Directory
	err     error
	token   string
	users   []User
	queried int
	created []NewAccount
}

func (f *fakeDirectory) SignIn(context.Context, string, string) (string, error) {
	return f.token, f.err
}
func (f *fakeDirectory) SignUp(context.Context, string, string, string) error { return f.err }
func (f *fakeDirectory) ConfirmSignUp(context.Context, string, string) error  { return f.err }
func (f *fakeDirectory) ResendCode(context.Context, string) error             { return f.err }
func (f *fakeDirectory) ListUsers(context.Context) ([]User, error) {
	return append([]User(nil), f.users...), f.err
}
func (f *fakeDirectory) CreateAccount(_ context.Context, name, email string, role actor.Role) error {
	f.created = append(f.created, NewAccount{Name: name, Email: email, Role: role})
	return f.err
}
func (f *fakeDirectory) UsersWithSub(_ context.Context, sub string) ([]User, error) {
	f.queried++
	var out []User
	for _, u := range f.users {
		if u.ID == sub {
			out = append(out, u)
		}
	}
	return out, nil
}

type cohorts map[string]string

func (c cohorts) CohortOf(_ context.Context, rgm string) (string, error) { return c[rgm], nil }

func newTestService(t *testing.T, d *fakeDirectory) *Service {
	t.Helper()
	t.Cleanup(audit.SetOutput(io.Discard))
	return NewService(d, cohorts{"12345678901": "turma-1"})
}

func token(claims map[string]any) string {
	payload, _ := json.Marshal(claims)
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none"}`)) + "." + enc(payload) + ".sig"
}

func status(err error) (apperr.Kind, string) {
	e, _ := apperr.As(err)
	if e == nil {
		return 0, ""
	}
	return e.Kind, e.Message
}

func TestLogin(t *testing.T) {
	d := &fakeDirectory{token: token(map[string]any{"sub": "s1", "email": "12345678901@alunos.umc.br", "name": "Ana"})}
	s, err := newTestService(t, d).Login(context.Background(), Credentials{Email: " 12345678901@ALUNOS.umc.br ", Password: "x"})
	want := SessionUser{ID: "s1", Name: "Ana", Email: "12345678901@alunos.umc.br", Role: actor.RoleStudent, Status: StatusActive,
		CohortIDs: []string{"turma-1"}, RGM: "12345678901"}
	if err != nil || !reflect.DeepEqual(s.User, want) || s.Token != d.token {
		t.Fatalf("session = %+v, err = %v", s, err)
	}

	for _, c := range []struct {
		name string
		in   Credentials
		err  error
		kind apperr.Kind
	}{
		{"missing password", Credentials{Email: "a@b"}, nil, apperr.KindInvalid},
		{"not confirmed", Credentials{Email: "a@b", Password: "x"}, errNotConfirmed, apperr.KindForbidden},
		{"bad credentials", Credentials{Email: "a@b", Password: "x"}, errors.New("NotAuthorized"), apperr.KindUnauthorized},
	} {
		_, err := newTestService(t, &fakeDirectory{err: c.err}).Login(context.Background(), c.in)
		if k, _ := status(err); k != c.kind {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}

func TestSignUpErrorsHaveTheirOwnMessage(t *testing.T) {
	cases := map[error]apperr.Kind{
		errWeakPassword:       apperr.KindInvalid,
		errEmailTaken:         apperr.KindConflict,
		errSignUpRejected:     apperr.KindForbidden,
		errInvalidInput:       apperr.KindInvalid,
		errThrottled:          apperr.KindTooManyRequests,
		errors.New("timeout"): apperr.KindInvalid,
	}
	messages := map[string]bool{}
	for cause, kind := range cases {
		err := newTestService(t, &fakeDirectory{err: cause}).SignUp(context.Background(), SignUpInput{Name: "A", Email: "1@alunos.umc.br", Password: "x"})
		k, msg := status(err)
		if k != kind || msg == "" {
			t.Errorf("%v: kind %d, message %q", cause, k, msg)
		}
		messages[msg] = true
	}
	if len(messages) != len(cases) {
		t.Errorf("want %d distinct messages, got %d", len(cases), len(messages))
	}
}

func TestConfirmValidatesInput(t *testing.T) {
	svc := newTestService(t, &fakeDirectory{err: errors.New("must not be called")})
	for _, in := range []Confirmation{{Email: "123@alunos.umc.br"}, {Code: "123456"}, {Email: " ", Code: " "}} {
		if k, msg := status(svc.Confirm(context.Background(), in)); k != apperr.KindInvalid || msg != msgConfirmRequired {
			t.Errorf("%+v: %d %q", in, k, msg)
		}
	}
	if k, _ := status(newTestService(t, &fakeDirectory{err: errAlreadyConfirmed}).Confirm(context.Background(), Confirmation{Email: "a@b", Code: "1"})); k != apperr.KindConflict {
		t.Errorf("already confirmed: kind %d", k)
	}
}

func TestUserFromAttributes(t *testing.T) {
	cases := []struct {
		name      string
		attrs     map[string]string
		username  string
		enabled   bool
		confirmed bool
		want      User
	}{
		{"self-registered student: no custom:perfil, RGM from the e-mail",
			map[string]string{"sub": "s1", "email": "12345678901@alunos.umc.br", "name": "Aluno"}, "s1", true, true,
			User{ID: "s1", Name: "Aluno", Email: "12345678901@alunos.umc.br", Role: actor.RoleStudent, Status: StatusActive, Confirmed: true, CohortIDs: []string{}, RGM: "12345678901"}},
		{"admin has no RGM",
			map[string]string{"sub": "s2", "email": "coordenacao@umc.br", "name": "Coordenação", "custom:perfil": "ADMIN"}, "s2", true, true,
			User{ID: "s2", Name: "Coordenação", Email: "coordenacao@umc.br", Role: actor.RoleAdmin, Status: StatusActive, Confirmed: true, CohortIDs: []string{}}},
		{"disabled, unconfirmed, id falls back to the username",
			map[string]string{"email": "22222222@alunos.umc.br", "name": "Fulano"}, "s3", false, false,
			User{ID: "s3", Name: "Fulano", Email: "22222222@alunos.umc.br", Role: actor.RoleStudent, Status: StatusInactive, CohortIDs: []string{}, RGM: "22222222"}},
	}
	for _, c := range cases {
		if got := userFromAttributes(c.attrs, c.username, c.enabled, c.confirmed); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
}

func TestListUsers(t *testing.T) {
	d := &fakeDirectory{users: []User{
		{ID: "2", Name: "coord", Email: "coord@umc.br", Role: actor.RoleAdmin, CohortIDs: []string{}},
		{ID: "1", Name: "Aluno", Email: "12345678901@alunos.umc.br", Role: actor.RoleStudent, RGM: "12345678901", CohortIDs: []string{}},
	}}
	for role, seesEmail := range map[actor.Role]bool{actor.RoleAdmin: true, actor.RoleAdvisor: true, actor.RoleStudent: false, "": false} {
		users, err := newTestService(t, d).ListUsers(context.Background(), actor.Actor{Role: role})
		if err != nil || len(users) != 2 || users[0].Name != "Aluno" || !reflect.DeepEqual(users[0].CohortIDs, []string{"turma-1"}) {
			t.Fatalf("%q: users = %+v, err = %v", role, users, err)
		}
		for _, u := range users {
			if (u.Email != "") != seesEmail {
				t.Errorf("%q: e-mail of %s = %q", role, u.Name, u.Email)
			}
		}
	}
	if _, err := newTestService(t, &fakeDirectory{err: errors.New("AccessDenied")}).ListUsers(context.Background(), actor.Actor{}); err == nil {
		t.Error("directory failure must fail")
	}
}

func TestCreateAccount(t *testing.T) {
	ctx := context.Background()
	d := &fakeDirectory{}
	if err := newTestService(t, d).CreateAccount(ctx, NewAccount{Name: " Prof ", Email: " Prof@UMC.br ", Role: actor.RoleAdvisor}); err != nil ||
		len(d.created) != 1 || d.created[0] != (NewAccount{Name: "Prof", Email: "prof@umc.br", Role: actor.RoleAdvisor}) {
		t.Fatalf("created = %+v, err = %v", d.created, err)
	}
	for name, in := range map[string]NewAccount{
		"student domain": {Name: "A", Email: "1@alunos.umc.br", Role: actor.RoleAdvisor},
		"student role":   {Name: "A", Email: "a@umc.br", Role: actor.RoleStudent},
		"missing name":   {Email: "a@umc.br", Role: actor.RoleAdmin},
	} {
		if k, _ := status(newTestService(t, &fakeDirectory{}).CreateAccount(ctx, in)); k != apperr.KindInvalid {
			t.Errorf("%s: kind %d", name, k)
		}
	}
	if k, msg := status(newTestService(t, &fakeDirectory{err: errEmailTaken}).CreateAccount(ctx, NewAccount{Name: "A", Email: "a@umc.br", Role: actor.RoleAdmin})); k != apperr.KindConflict || !strings.Contains(msg, "já existe") {
		t.Errorf("taken: %d %q", k, msg)
	}
}

func TestAdvisorExists(t *testing.T) {
	const sub = "0a1b2c3d-0000-4000-8000-0123456789ab"
	const admin = "11111111-0000-4000-8000-000000000000"
	d := &fakeDirectory{users: []User{{ID: sub, Role: actor.RoleAdvisor}, {ID: admin, Role: actor.RoleAdmin}}}
	svc := newTestService(t, d)
	for id, want := range map[string]bool{sub: true, admin: false, "22222222-0000-4000-8000-000000000000": false} {
		if got, err := svc.AdvisorExists(context.Background(), id); err != nil || got != want {
			t.Errorf("AdvisorExists(%q) = %v, %v", id, got, err)
		}
	}
	d.queried = 0
	if ok, _ := svc.AdvisorExists(context.Background(), `x" or email ^= "`); ok || d.queried != 0 {
		t.Errorf("an id outside the sub format reached Cognito (%d queries)", d.queried)
	}
}
