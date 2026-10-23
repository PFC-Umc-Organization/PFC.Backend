package integracao

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const (
	validPassword    = "Senha#123"
	confirmationCode = "123456"
)

type fakeUser struct {
	sub       string
	email     string
	password  string
	attrs     map[string]string
	confirmed bool
	enabled   bool
	forced    bool
}

// fakeCognito implements the subset of the Cognito JSON protocol the backend calls.
type fakeCognito struct {
	mu    sync.Mutex
	users map[string]*fakeUser
	next  int
}

func newFakeCognito() *fakeCognito {
	return &fakeCognito{users: map[string]*fakeUser{}}
}

func (f *fakeCognito) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users = map[string]*fakeUser{}
	f.next = 0
}

// subOf is the sub Cognito puts in the token of the account, as the
// authorizer would.
func (f *fakeCognito) subOf(email string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.users[email]; ok {
		return u.sub
	}
	return "sub-" + email
}

func (f *fakeCognito) add(email, name, role string, confirmed bool) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.addLocked(email, validPassword, map[string]string{"name": name, "custom:perfil": role}, confirmed, false)
}

func (f *fakeCognito) addLocked(email, password string, attrs map[string]string, confirmed, forced bool) string {
	f.next++
	sub := fmt.Sprintf("00000000-0000-4000-8000-%012d", f.next)
	attrs["email"] = email
	attrs["sub"] = sub
	for k, v := range attrs {
		if v == "" {
			delete(attrs, k)
		}
	}
	f.users[email] = &fakeUser{sub: sub, email: email, password: password, attrs: attrs, confirmed: confirmed, enabled: true, forced: forced}
	return sub
}

func (f *fakeCognito) serve(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", u.Host)
	if err != nil {
		return err
	}
	go http.Serve(ln, f)
	return nil
}

func (f *fakeCognito) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		f.fail(w, "InvalidParameterException", "bad json")
		return
	}
	op := strings.TrimPrefix(r.Header.Get("X-Amz-Target"), "AWSCognitoIdentityProviderService.")

	f.mu.Lock()
	defer f.mu.Unlock()
	switch op {
	case "InitiateAuth":
		f.initiateAuth(w, in)
	case "SignUp":
		f.signUp(w, in)
	case "ConfirmSignUp":
		f.confirmSignUp(w, in)
	case "ResendConfirmationCode":
		f.resend(w, in)
	case "ListUsers":
		f.listUsers(w, in)
	case "AdminCreateUser":
		f.adminCreate(w, in)
	case "RespondToAuthChallenge":
		f.respondChallenge(w, in)
	case "ForgotPassword":
		f.forgotPassword(w, in)
	case "ConfirmForgotPassword":
		f.confirmForgotPassword(w, in)
	default:
		f.fail(w, "InvalidParameterException", "unsupported operation "+op)
	}
}

func (f *fakeCognito) ok(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	json.NewEncoder(w).Encode(body)
}

func (f *fakeCognito) fail(w http.ResponseWriter, code, msg string) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.Header().Set("X-Amzn-ErrorType", code)
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": msg})
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func (f *fakeCognito) initiateAuth(w http.ResponseWriter, in map[string]any) {
	params, _ := in["AuthParameters"].(map[string]any)
	u := f.users[str(params, "USERNAME")]
	switch {
	case u == nil || u.password != str(params, "PASSWORD"):
		f.fail(w, "NotAuthorizedException", "Incorrect username or password.")
	case !u.confirmed:
		f.fail(w, "UserNotConfirmedException", "User is not confirmed.")
	case u.forced:
		f.ok(w, map[string]any{"ChallengeName": "NEW_PASSWORD_REQUIRED", "Session": "sessao-" + u.sub})
	default:
		f.ok(w, map[string]any{"AuthenticationResult": map[string]any{"IdToken": idToken(u.attrs)}})
	}
}

func idToken(attrs map[string]string) string {
	payload, _ := json.Marshal(attrs)
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none"}`)) + "." + enc(payload) + ".assinatura"
}

var studentEmail = regexp.MustCompile(`^\d+@alunos\.umc\.br$`)

func (f *fakeCognito) signUp(w http.ResponseWriter, in map[string]any) {
	email := str(in, "Username")
	attrs := map[string]string{}
	list, _ := in["UserAttributes"].([]any)
	for _, a := range list {
		m, _ := a.(map[string]any)
		attrs[str(m, "Name")] = str(m, "Value")
	}
	switch {
	case f.users[email] != nil:
		f.fail(w, "UsernameExistsException", "User already exists")
	case len(str(in, "Password")) < 8:
		f.fail(w, "InvalidPasswordException", "Password did not conform with policy")
	case !studentEmail.MatchString(email):
		f.fail(w, "UserLambdaValidationException", "PreSignUp failed with error RGM não autorizado.")
	default:
		sub := f.addLocked(email, str(in, "Password"), attrs, false, false)
		f.ok(w, map[string]any{"UserConfirmed": false, "UserSub": sub})
	}
}

func (f *fakeCognito) confirmSignUp(w http.ResponseWriter, in map[string]any) {
	u := f.users[str(in, "Username")]
	switch {
	case u == nil:
		f.fail(w, "UserNotFoundException", "Username/client id combination not found.")
	case u.confirmed:
		f.fail(w, "NotAuthorizedException", "User cannot be confirmed. Current status is CONFIRMED")
	case str(in, "ConfirmationCode") != confirmationCode:
		f.fail(w, "CodeMismatchException", "Invalid verification code provided, please try again.")
	default:
		u.confirmed = true
		f.ok(w, map[string]any{})
	}
}

func (f *fakeCognito) resend(w http.ResponseWriter, in map[string]any) {
	if f.users[str(in, "Username")] == nil {
		f.fail(w, "UserNotFoundException", "Username/client id combination not found.")
		return
	}
	f.ok(w, map[string]any{"CodeDeliveryDetails": map[string]any{"DeliveryMedium": "EMAIL"}})
}

var listFilter = regexp.MustCompile(`^(\w+) (\^?=) "(.*)"$`)

func (f *fakeCognito) listUsers(w http.ResponseWriter, in map[string]any) {
	var attr, op, value string
	if m := listFilter.FindStringSubmatch(str(in, "Filter")); m != nil {
		attr, op, value = m[1], m[2], m[3]
	}
	emails := make([]string, 0, len(f.users))
	for e := range f.users {
		emails = append(emails, e)
	}
	sort.Strings(emails)

	users := []any{}
	for _, e := range emails {
		u := f.users[e]
		if attr != "" {
			v := u.attrs[attr]
			if (op == "=" && v != value) || (op == "^=" && !strings.HasPrefix(v, value)) {
				continue
			}
		}
		attrs := []any{}
		names := make([]string, 0, len(u.attrs))
		for n := range u.attrs {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			attrs = append(attrs, map[string]string{"Name": n, "Value": u.attrs[n]})
		}
		status := "UNCONFIRMED"
		if u.confirmed {
			status = "CONFIRMED"
		}
		if u.forced {
			status = "FORCE_CHANGE_PASSWORD"
		}
		users = append(users, map[string]any{"Username": u.sub, "Attributes": attrs, "Enabled": u.enabled, "UserStatus": status})
	}
	f.ok(w, map[string]any{"Users": users})
}

func (f *fakeCognito) adminCreate(w http.ResponseWriter, in map[string]any) {
	email := str(in, "Username")
	if f.users[email] != nil {
		f.fail(w, "UsernameExistsException", "User account already exists")
		return
	}
	attrs := map[string]string{}
	list, _ := in["UserAttributes"].([]any)
	for _, a := range list {
		m, _ := a.(map[string]any)
		if n := str(m, "Name"); n != "email_verified" {
			attrs[n] = str(m, "Value")
		}
	}
	sub := f.addLocked(email, "temporaria", attrs, true, true)
	f.ok(w, map[string]any{"User": map[string]any{"Username": sub, "Enabled": true, "UserStatus": "FORCE_CHANGE_PASSWORD"}})
}

func (f *fakeCognito) respondChallenge(w http.ResponseWriter, in map[string]any) {
	answers, _ := in["ChallengeResponses"].(map[string]any)
	u := f.users[str(answers, "USERNAME")]
	switch {
	case u == nil || !u.forced || str(in, "Session") != "sessao-"+u.sub:
		f.fail(w, "NotAuthorizedException", "Invalid session for the user, session is expired.")
	case len(str(answers, "NEW_PASSWORD")) < 8:
		f.fail(w, "InvalidPasswordException", "Password does not conform to policy")
	default:
		u.password, u.forced = str(answers, "NEW_PASSWORD"), false
		f.ok(w, map[string]any{"AuthenticationResult": map[string]any{"IdToken": idToken(u.attrs)}})
	}
}

func (f *fakeCognito) forgotPassword(w http.ResponseWriter, in map[string]any) {
	u := f.users[str(in, "Username")]
	switch {
	case u == nil:
		f.fail(w, "UserNotFoundException", "Username/client id combination not found.")
	case !u.confirmed:
		f.fail(w, "InvalidParameterException", "Cannot reset password for the user as there is no registered/verified email or phone_number")
	default:
		f.ok(w, map[string]any{"CodeDeliveryDetails": map[string]any{"DeliveryMedium": "EMAIL"}})
	}
}

func (f *fakeCognito) confirmForgotPassword(w http.ResponseWriter, in map[string]any) {
	u := f.users[str(in, "Username")]
	switch {
	case u == nil:
		f.fail(w, "UserNotFoundException", "Username/client id combination not found.")
	case str(in, "ConfirmationCode") != confirmationCode:
		f.fail(w, "CodeMismatchException", "Invalid verification code provided, please try again.")
	case len(str(in, "Password")) < 8:
		f.fail(w, "InvalidPasswordException", "Password does not conform to policy")
	default:
		u.password = str(in, "Password")
		f.ok(w, map[string]any{})
	}
}
