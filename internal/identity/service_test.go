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
	"time"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

type fakeDirectory struct {
	Directory
	err       error
	token     string
	challenge string
	users     []User
	queried   int
	created   []NewAccount
}

func (f *fakeDirectory) SignIn(context.Context, string, string) (SignInResult, error) {
	return SignInResult{IDToken: f.token, NewPasswordSession: f.challenge}, f.err
}
func (f *fakeDirectory) CompleteNewPassword(context.Context, string, string, string) (string, error) {
	return f.token, f.err
}
func (f *fakeDirectory) ForgotPassword(context.Context, string) error { return f.err }
func (f *fakeDirectory) ConfirmForgotPassword(context.Context, string, string, string) error {
	return f.err
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
func (f *fakeDirectory) UsersWithEmailPrefix(_ context.Context, prefix string) ([]User, error) {
	f.queried++
	var out []User
	for _, u := range f.users {
		if u.RGM != "" && strings.HasPrefix(u.RGM+"@", prefix) {
			out = append(out, u)
		}
	}
	return out, f.err
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
func (c cohorts) CohortsOf(_ context.Context, rgms []string) (map[string]string, error) {
	out := map[string]string{}
	for _, r := range rgms {
		if c[r] != "" {
			out[r] = c[r]
		}
	}
	return out, nil
}

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
	if err != nil || s.User == nil || !reflect.DeepEqual(*s.User, want) || s.Token != d.token {
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
}

func TestConfirmDoesNotRevealAccounts(t *testing.T) {
	for _, err := range []error{errWrongCode, errNoAccount, errAlreadyConfirmed} {
		k, msg := status(newTestService(t, &fakeDirectory{err: err}).Confirm(context.Background(), Confirmation{Email: "a@b", Code: "1"}))
		if k != apperr.KindInvalid || msg != msgConfirmFailed {
			t.Errorf("%v: %d %q", err, k, msg)
		}
	}
	for _, err := range []error{nil, errNoAccount, errInvalidInput} {
		if got := newTestService(t, &fakeDirectory{err: err}).ResendCode(context.Background(), ResendInput{Email: "a@b"}); got != nil {
			t.Errorf("resend %v: %v", err, got)
		}
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
		{ID: "2", Name: "coord", Email: "coord@umc.br", Role: actor.RoleAdmin},
		{ID: "1", Name: "Aluno", Email: "12345678901@alunos.umc.br", Role: actor.RoleStudent, RGM: "12345678901"},
	}}
	users, err := newTestService(t, d).ListUsers(context.Background())
	if err != nil || len(users) != 2 || users[0].Name != "Aluno" || !reflect.DeepEqual(users[0].CohortIDs, []string{"turma-1"}) ||
		!reflect.DeepEqual(users[1].CohortIDs, []string{}) || users[1].Email != "coord@umc.br" {
		t.Fatalf("users = %+v, err = %v", users, err)
	}
	if _, err := newTestService(t, &fakeDirectory{err: errors.New("AccessDenied")}).ListUsers(context.Background()); err == nil {
		t.Error("directory failure must fail")
	}
}

func TestStudentAndAdvisorNames(t *testing.T) {
	ctx := context.Background()
	sub := "0c8f5a7e-1111-4222-8333-944455556666"
	d := &fakeDirectory{users: []User{
		{ID: "s1", Name: "Ana", Role: actor.RoleStudent, RGM: "11111111111"},
		{ID: "s2", Name: "Prof com RGM", Role: actor.RoleAdvisor, RGM: "22222222222"},
		{ID: sub, Name: "Orientadora", Role: actor.RoleAdvisor},
	}}
	svc := newTestService(t, d)
	names, err := svc.StudentNames(ctx, []string{"11111111111", "22222222222", "33333333333"})
	if err != nil || !reflect.DeepEqual(names, map[string]string{"11111111111": "Ana"}) {
		t.Fatalf("names = %v, err = %v", names, err)
	}
	if name, ok, err := svc.AdvisorName(ctx, sub); err != nil || !ok || name != "Orientadora" {
		t.Fatalf("advisor = %q %v %v", name, ok, err)
	}
	if _, ok, _ := svc.AdvisorName(ctx, "s1"); ok {
		t.Error("a malformed sub is not an advisor")
	}
}

func TestNamesAreCachedUntilTheTTL(t *testing.T) {
	ctx := context.Background()
	sub := "0c8f5a7e-1111-4222-8333-944455556666"
	d := &fakeDirectory{users: []User{
		{ID: "s1", Name: "Ana", Role: actor.RoleStudent, RGM: "11111111111"},
		{ID: sub, Name: "Orientadora", Role: actor.RoleAdvisor},
	}}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	svc := newTestService(t, d)
	svc.names = newNameCache(func() time.Time { return now })
	lookup := func() {
		t.Helper()
		names, err := svc.StudentNames(ctx, []string{"11111111111", "33333333333"})
		name, _, err2 := svc.AdvisorName(ctx, sub)
		if err != nil || err2 != nil || names["11111111111"] != "Ana" || name != "Orientadora" {
			t.Fatalf("names = %v, advisor = %q, err = %v %v", names, name, err, err2)
		}
	}
	lookup()
	if d.queried != 3 {
		t.Fatalf("first lookup made %d queries, want 3", d.queried)
	}
	d.users[0].Name = "Ana Souza"
	lookup()
	if d.queried != 4 {
		t.Errorf("a cached name was looked up again, or a missing one was cached (%d queries)", d.queried)
	}
	now = now.Add(nameTTL)
	names, _ := svc.StudentNames(ctx, []string{"11111111111"})
	if names["11111111111"] != "Ana Souza" {
		t.Errorf("after the TTL the name is %q", names["11111111111"])
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
	if k, msg := status(newTestService(t, &fakeDirectory{err: errEmailTaken}).CreateAccount(ctx, NewAccount{Name: "A", Email: "a@umc.br", Role: actor.RoleAdmin})); k != apperr.KindConflict || !strings.Contains(msg, "Já existe") {
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

func TestLoginWithTemporaryPassword(t *testing.T) {
	res, err := newTestService(t, &fakeDirectory{challenge: "sess"}).Login(context.Background(), Credentials{Email: "prof@umc.br", Password: "Temp#123"})
	if err != nil || res.Challenge != ChallengeNewPassword || res.Session != "sess" || res.User != nil || res.Token != "" {
		t.Fatalf("result = %+v, err = %v", res, err)
	}
}

func TestCompleteNewPassword(t *testing.T) {
	tok := token(map[string]any{"sub": "s9", "email": "prof@umc.br", "name": "Prof", "custom:perfil": "ORIENTADOR"})
	s, err := newTestService(t, &fakeDirectory{token: tok}).CompleteNewPassword(context.Background(), NewPasswordInput{Email: " Prof@UMC.br", Session: "sess", Password: "Nova#1234"})
	if err != nil || s.User.ID != "s9" || s.User.Role != actor.RoleAdvisor || s.Token != tok {
		t.Fatalf("session = %+v, err = %v", s, err)
	}
	for cause, want := range map[error]apperr.Kind{errWeakPassword: apperr.KindInvalid, errSessionExpired: apperr.KindUnauthorized, errThrottled: apperr.KindTooManyRequests} {
		_, err := newTestService(t, &fakeDirectory{err: cause}).CompleteNewPassword(context.Background(), NewPasswordInput{Email: "a@umc.br", Session: "s", Password: "x"})
		if k, _ := status(err); k != want {
			t.Errorf("%v: kind %d", cause, k)
		}
	}
	if _, err := newTestService(t, &fakeDirectory{}).CompleteNewPassword(context.Background(), NewPasswordInput{Email: "a@umc.br"}); err == nil {
		t.Error("missing session must be refused")
	}
}

func TestForgotPasswordDoesNotRevealAccounts(t *testing.T) {
	for _, cause := range []error{nil, errNoAccount, errInvalidInput} {
		if err := newTestService(t, &fakeDirectory{err: cause}).ForgotPassword(context.Background(), ForgotPasswordInput{Email: "x@umc.br"}); err != nil {
			t.Errorf("%v: %v", cause, err)
		}
	}
	if k, _ := status(newTestService(t, &fakeDirectory{err: errThrottled}).ForgotPassword(context.Background(), ForgotPasswordInput{Email: "x@umc.br"})); k != apperr.KindTooManyRequests {
		t.Error("throttling must surface")
	}
}

func TestResetPassword(t *testing.T) {
	ctx := context.Background()
	in := ResetPasswordInput{Email: "a@umc.br", Code: "123456", Password: "Nova#1234"}
	if err := newTestService(t, &fakeDirectory{}).ResetPassword(ctx, in); err != nil {
		t.Fatal(err)
	}
	for cause, msg := range map[error]string{errWrongCode: msgWrongCode, errNoAccount: msgWrongCode, errExpiredCode: msgExpiredCode, errWeakPassword: msgWeakPassword} {
		if _, got := status(newTestService(t, &fakeDirectory{err: cause}).ResetPassword(ctx, in)); got != msg {
			t.Errorf("%v: %q", cause, got)
		}
	}
	if _, msg := status(newTestService(t, &fakeDirectory{}).ResetPassword(ctx, ResetPasswordInput{Email: "a@umc.br"})); msg != msgResetRequired {
		t.Errorf("missing fields: %q", msg)
	}
}

func TestMe(t *testing.T) {
	svc := newTestService(t, &fakeDirectory{})
	u, err := svc.Me(context.Background(), actor.Actor{Sub: "s1", Name: "Ana", Email: "12345678901@alunos.umc.br"})
	want := SessionUser{ID: "s1", Name: "Ana", Email: "12345678901@alunos.umc.br", Role: actor.RoleStudent, Status: StatusActive, CohortIDs: []string{"turma-1"}, RGM: "12345678901"}
	if err != nil || !reflect.DeepEqual(u, want) {
		t.Fatalf("me = %+v, err = %v", u, err)
	}
	if _, err := svc.Me(context.Background(), actor.Actor{}); err == nil {
		t.Error("no sub must be refused")
	}
}
