package groups

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

// world is an in-memory store plus academic data. Invitations are keyed
// "<projectID>|<rgm>".
type world struct {
	Store
	projects    map[string]Project
	taken       map[string]string // rgm -> project id
	invitations map[string]bool
	cohorts     map[string]string // rgm -> cohort
	programs    map[string]string // cohort -> program
	full        bool
	joined      []string
}

func newWorld() *world {
	return &world{
		projects:    map[string]Project{"p1": {ID: "p1", Name: "Athena", ProgramID: "prog-a", Members: []string{"111"}}},
		taken:       map[string]string{"111": "p1"},
		invitations: map[string]bool{"p1|222": true, "p2|222": true},
		cohorts:     map[string]string{"111": "turma-a", "222": "turma-a", "333": "turma-b", "444": "turma-a", "555": "turma-a"},
		programs:    map[string]string{"turma-a": "prog-a", "turma-b": "prog-b"},
	}
}

func (w *world) GetProject(_ context.Context, id string) (Project, bool, error) {
	p, ok := w.projects[id]
	p.Members = slices.Clone(p.Members)
	return p, ok, nil
}
func (w *world) CreateProject(_ context.Context, p Project) error {
	for _, m := range p.Members {
		if _, ok := w.taken[m]; ok {
			return errMemberTaken
		}
	}
	w.projects[p.ID] = p
	return nil
}
func (w *world) ProjectOfMember(ctx context.Context, rgm string) (Project, bool, error) {
	id, ok := w.taken[rgm]
	if !ok {
		return Project{}, false, nil
	}
	return w.GetProject(ctx, id)
}
func (w *world) ProjectsOfMembers(_ context.Context, rgms []string) (map[string]string, error) {
	out := map[string]string{}
	for _, r := range rgms {
		if id, ok := w.taken[r]; ok {
			out[r] = id
		}
	}
	return out, nil
}
func (w *world) AddMember(_ context.Context, id, rgm string, _ []string) error {
	if w.full {
		return errStale
	}
	w.joined = append(w.joined, id+"|"+rgm)
	return nil
}
func (w *world) CreateInvitation(_ context.Context, inv Invitation) error {
	key := inv.ProjectID + "|" + inv.RGM
	if w.invitations[key] {
		return errDuplicate
	}
	w.invitations[key] = true
	return nil
}
func (w *world) InvitationExists(_ context.Context, projectID, rgm string) (bool, error) {
	return w.invitations[projectID+"|"+rgm], nil
}
func (w *world) invitationsWhere(match func(project, rgm string) bool) []Invitation {
	out := []Invitation{}
	for key := range w.invitations {
		project, rgm, _ := strings.Cut(key, "|")
		if match(project, rgm) {
			out = append(out, Invitation{ProjectID: project, RGM: rgm})
		}
	}
	return out
}
func (w *world) InvitationsOfProject(_ context.Context, projectID string) ([]Invitation, error) {
	return w.invitationsWhere(func(p, _ string) bool { return p == projectID }), nil
}
func (w *world) InvitationsOfStudent(_ context.Context, rgm string) ([]Invitation, error) {
	return w.invitationsWhere(func(_, r string) bool { return r == rgm }), nil
}
func (w *world) DeleteInvitation(_ context.Context, projectID, rgm string) error {
	key := projectID + "|" + rgm
	if !w.invitations[key] {
		return errNotFound
	}
	delete(w.invitations, key)
	return nil
}

func (w *world) ProgramExists(context.Context, string) (bool, error) { return true, nil }
func (w *world) CohortOfProgram(_ context.Context, program string) (string, bool, error) {
	for cohort, p := range w.programs {
		if p == program {
			return cohort, true, nil
		}
	}
	return "", false, nil
}
func (w *world) Cohort(_ context.Context, id string) (Cohort, bool, error) {
	if id == "" {
		return Cohort{}, false, nil
	}
	return Cohort{ID: id, Course: "Curso " + id, Shift: "NOITE", Term: "2026.2"}, true, nil
}
func (w *world) ProjectsOfProgram(_ context.Context, program string) ([]Project, error) {
	out := []Project{}
	for _, p := range w.projects {
		if p.ProgramID == program {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b Project) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}
func (w *world) ProgramOfCohort(_ context.Context, cohort string) (string, bool, error) {
	id, ok := w.programs[cohort]
	return id, ok, nil
}
func (w *world) IsEnrolled(_ context.Context, rgm string) (bool, error) {
	_, ok := w.cohorts[rgm]
	return ok, nil
}
func (w *world) ProgramIDs(context.Context) ([]string, error) {
	ids := []string{}
	for _, p := range w.programs {
		ids = append(ids, p)
	}
	return ids, nil
}
func (w *world) CohortOf(_ context.Context, rgm string) (string, error) { return w.cohorts[rgm], nil }
func (w *world) CohortsOf(_ context.Context, rgms []string) (map[string]string, error) {
	out := map[string]string{}
	for _, r := range rgms {
		if c, ok := w.cohorts[r]; ok {
			out[r] = c
		}
	}
	return out, nil
}

type noAccounts struct{}

func (noAccounts) StudentExists(context.Context, string) (bool, error) { return false, nil }
func (noAccounts) AdvisorExists(context.Context, string) (bool, error) { return false, nil }
func (noAccounts) StudentNames(_ context.Context, rgms []string) (map[string]string, error) {
	out := map[string]string{}
	for _, r := range rgms {
		if r != "999" {
			out[r] = "Aluno " + r
		}
	}
	return out, nil
}
func (noAccounts) AdvisorName(_ context.Context, sub string) (string, bool, error) {
	return "Prof " + sub, true, nil
}

func (w *world) service(t *testing.T) (*Service, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	t.Cleanup(audit.SetOutput(&buf))
	svc := NewService(w, w, noAccounts{})
	svc.newID = func() string { return "new" }
	svc.now = func() time.Time { return time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC) }
	svc.notify = func(context.Context, Invitation) {}
	return svc, &buf
}

func studentActor(rgm string) actor.Actor {
	return actor.Actor{Role: actor.RoleStudent, Email: rgm + "@alunos.umc.br", Name: "Aluno " + rgm}
}

var advisor = actor.Actor{Role: actor.RoleAdvisor, Email: "prof@umc.br", Sub: "prof-sub"}

func kindOf(err error) apperr.Kind {
	if e, ok := apperr.As(err); ok {
		return e.Kind
	}
	return 0
}

func TestGroup(t *testing.T) {
	if got := group("1", []string{"2", " 2 ", "", "1", "3"}); !slices.Equal(got, []string{"1", "2", "3"}) {
		t.Errorf("group = %v", got)
	}
}

func TestCreateOwnProject(t *testing.T) {
	w := newWorld()
	delete(w.taken, "111")
	svc, buf := w.service(t)

	p, err := svc.CreateOwnProject(context.Background(), studentActor("111"),
		ProjectInput{Name: " Novo ", Description: "d", Members: []string{"222", "111", " 222 ", ""}})
	if err != nil {
		t.Fatal(err)
	}
	if p.ProgramID != "prog-a" || p.Name != "Novo" || !slices.Equal(p.Members, []string{"111"}) {
		t.Errorf("project = %+v", p)
	}
	if !w.invitations["new|222"] || len(w.invitationsWhere(func(p, _ string) bool { return p == "new" })) != 1 {
		t.Errorf("invitations = %v", w.invitations)
	}
	if !strings.Contains(buf.String(), `"convidados":["222"]`) {
		t.Errorf("audit = %s", buf)
	}
}

func TestCreateOwnProjectRefusals(t *testing.T) {
	cases := []struct {
		name    string
		rgm     string
		in      ProjectInput
		setup   func(*world)
		kind    apperr.Kind
		message string
	}{
		{"blank name", "444", ProjectInput{Name: "  "}, nil, apperr.KindInvalid, msgNameRequired},
		{"no cohort", "999", ProjectInput{Name: "A"}, nil, apperr.KindForbidden, msgNoCohort},
		{"cohort without program", "333", ProjectInput{Name: "A"}, func(w *world) { delete(w.programs, "turma-b") }, apperr.KindConflict, msgCohortWithoutProgram},
		{"creator taken", "111", ProjectInput{Name: "A"}, nil, apperr.KindConflict, msgAlreadyInProject},
		{"classmate from another cohort", "444", ProjectInput{Name: "A", Members: []string{"333"}}, nil, apperr.KindInvalid, msgNotInCohort("333")},
		{"classmate not enrolled", "444", ProjectInput{Name: "A", Members: []string{"888"}}, nil, apperr.KindInvalid, msgNotInCohort("888")},
		{"classmate taken", "444", ProjectInput{Name: "A", Members: []string{"111"}}, nil, apperr.KindConflict, msgInOtherProject("111")},
		{"group too large", "444", ProjectInput{Name: "A", Members: strings.Split("a b c d e f g h i j", " ")}, nil, apperr.KindInvalid, msgGroupTooLarge},
	}
	for _, c := range cases {
		w := newWorld()
		if c.setup != nil {
			c.setup(w)
		}
		before := len(w.invitations)
		svc, _ := w.service(t)
		_, err := svc.CreateOwnProject(context.Background(), studentActor(c.rgm), c.in)
		if e, ok := apperr.As(err); !ok || e.Kind != c.kind || e.Message != c.message {
			t.Errorf("%s: err = %v", c.name, err)
		}
		if _, created := w.projects["new"]; created || len(w.invitations) != before {
			t.Errorf("%s: must not create project or invitations", c.name)
		}
	}
}

func TestAcceptInvitation(t *testing.T) {
	w := newWorld()
	svc, _ := w.service(t)
	p, err := svc.AcceptInvitation(context.Background(), "222", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Members, []string{"111", "222"}) || !slices.Equal(w.joined, []string{"p1|222"}) {
		t.Errorf("members = %v, joined = %v", p.Members, w.joined)
	}
	if len(w.invitations) != 0 {
		t.Errorf("accepting must clear the student's invitations: %v", w.invitations)
	}
}

func TestAcceptInvitationRefusals(t *testing.T) {
	cases := []struct {
		name    string
		rgm     string
		project string
		setup   func(*world)
		message string
	}{
		{"no invitation", "333", "p1", nil, msgInvitationNotFound},
		{"group deleted", "222", "p2", nil, msgGroupGone},
		{"already in a project", "222", "p1", func(w *world) { w.taken["222"] = "p1" }, msgAlreadyInProject},
		{"changed cohort", "222", "p1", func(w *world) { w.cohorts["222"] = "turma-b" }, msgOutsideGroupCohort},
		{"group full", "222", "p1", func(w *world) { w.full = true }, msgCannotJoin},
	}
	for _, c := range cases {
		w := newWorld()
		if c.setup != nil {
			c.setup(w)
		}
		svc, _ := w.service(t)
		_, err := svc.AcceptInvitation(context.Background(), c.rgm, c.project)
		if e, ok := apperr.As(err); !ok || e.Message != c.message {
			t.Errorf("%s: err = %v", c.name, err)
		}
		if len(w.joined) != 0 {
			t.Errorf("%s: must not join", c.name)
		}
	}
}

func TestAcceptInvitationToDeletedGroupDropsIt(t *testing.T) {
	w := newWorld()
	svc, _ := w.service(t)
	_, _ = svc.AcceptInvitation(context.Background(), "222", "p2")
	if w.invitations["p2|222"] || !w.invitations["p1|222"] {
		t.Errorf("invitations = %v", w.invitations)
	}
}

func TestDeclineInvitation(t *testing.T) {
	w := newWorld()
	svc, _ := w.service(t)
	if err := svc.DeclineInvitation(context.Background(), "222", "p1"); err != nil || w.invitations["p1|222"] {
		t.Fatalf("err = %v, invitations = %v", err, w.invitations)
	}
	if err := svc.DeclineInvitation(context.Background(), "222", "p1"); kindOf(err) != apperr.KindNotFound {
		t.Errorf("declining twice: %v", err)
	}
}

func TestInvite(t *testing.T) {
	cases := []struct {
		name    string
		by      actor.Actor
		project string
		rgm     string
		setup   func(*world)
		message string
	}{
		{"member invites a free classmate", studentActor("111"), "p1", " 444 ", nil, ""},
		{"missing project", studentActor("111"), "p9", "444", nil, msgProjectNotFound},
		{"outsider", studentActor("222"), "p1", "444", nil, msgMembersOnlyInvite},
		{"advisor", advisor, "p1", "444", nil, msgMembersOnlyInvite},
		{"blank rgm", studentActor("111"), "p1", "  ", nil, msgRGMRequired},
		{"already a member", studentActor("111"), "p1", "111", nil, msgAlreadyMember("111")},
		{"other cohort", studentActor("111"), "p1", "333", nil, msgNotInGroupCohort("333")},
		{"classmate taken", studentActor("111"), "p1", "555", func(w *world) { w.taken["555"] = "p1" }, msgInOtherProject("555")},
		{"repeated invitation", studentActor("111"), "p1", "222", nil, msgAlreadyInvited("222")},
		{"full counting pending", studentActor("111"), "p1", "444", func(w *world) {
			p := w.projects["p1"]
			p.Members = []string{"111", "a", "b", "c", "d", "e", "f", "g", "h"}
			w.projects["p1"] = p
		}, msgGroupFullWithPending},
	}
	for _, c := range cases {
		w := newWorld()
		if c.setup != nil {
			c.setup(w)
		}
		before := len(w.invitations)
		svc, _ := w.service(t)
		inv, err := svc.Invite(context.Background(), c.by, c.project, MemberInput{RGM: c.rgm})
		if c.message == "" {
			if err != nil || !w.invitations["p1|444"] || inv.InvitedBy != "Aluno 111" || inv.CreatedAt != "2026-10-16T12:00:00Z" {
				t.Errorf("%s: inv = %+v, err = %v", c.name, inv, err)
			}
			continue
		}
		if e, ok := apperr.As(err); !ok || e.Message != c.message {
			t.Errorf("%s: err = %v", c.name, err)
		}
		if len(w.invitations) != before {
			t.Errorf("%s: must not create an invitation", c.name)
		}
	}
}

func TestInvitationsOfProjectVisibility(t *testing.T) {
	for viewer, allowed := range map[*actor.Actor]bool{
		ptr(studentActor("111")):                true,
		ptr(advisor):                            true,
		ptr(actor.Actor{Role: actor.RoleAdmin}): true,
		ptr(actor.Actor{Role: actor.RoleAdvisor, Email: "outro@umc.br", Sub: "outro"}): false,
		ptr(studentActor("333")): false,
	} {
		w := newWorld()
		w.advise("p1")
		svc, _ := w.service(t)
		invitations, err := svc.InvitationsOfProject(context.Background(), *viewer, "p1")
		if !allowed {
			if kindOf(err) != apperr.KindForbidden {
				t.Errorf("%s: err = %v", viewer.Email, err)
			}
			continue
		}
		if err != nil || len(invitations) != 1 || invitations[0].RGM != "222" || invitations[0].Name != "Aluno 222" {
			t.Errorf("%s: invitations = %+v, err = %v", viewer.Email, invitations, err)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func (w *world) advise(id string) {
	p := w.projects[id]
	p.AdvisorID = advisor.Sub
	w.projects[id] = p
}

func TestProjectsOfProgramScope(t *testing.T) {
	w := newWorld()
	w.projects["p2"] = Project{ID: "p2", Name: "Outro", ProgramID: "prog-a", Members: []string{}}
	w.advise("p2")
	svc, _ := w.service(t)
	ctx := context.Background()

	all, err := svc.ProjectsOfProgram(ctx, actor.Actor{Role: actor.RoleAdmin}, "prog-a")
	if err != nil || len(all) != 2 {
		t.Fatalf("admin: %+v, %v", all, err)
	}
	mine, err := svc.ProjectsOfProgram(ctx, advisor, "prog-a")
	if err != nil || len(mine) != 1 || mine[0].ID != "p2" {
		t.Fatalf("advisor: %+v, %v", mine, err)
	}
	none, _ := svc.ProjectsOfProgram(ctx, actor.Actor{Role: actor.RoleAdvisor}, "prog-a")
	if len(none) != 0 {
		t.Fatalf("advisor without sub sees %+v", none)
	}
}

func TestDetail(t *testing.T) {
	w := newWorld()
	p := w.projects["p1"]
	p.Members = []string{"111", "999"}
	w.projects["p1"] = p
	w.advise("p1")
	svc, _ := w.service(t)
	ctx := context.Background()

	d, err := svc.Detail(ctx, studentActor("111"), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Members) != 2 || d.Members[0].Name == nil || *d.Members[0].Name != "Aluno 111" || d.Members[1].Name != nil {
		t.Errorf("members = %+v", d.Members)
	}
	if d.Advisor == nil || *d.Advisor != (Advisor{ID: "prof-sub", Name: "Prof prof-sub"}) {
		t.Errorf("advisor = %+v", d.Advisor)
	}
	if d.Cohort == nil || d.Cohort.ID != "turma-a" {
		t.Errorf("cohort = %+v", d.Cohort)
	}
	if _, err := svc.Detail(ctx, studentActor("333"), "p1"); kindOf(err) != apperr.KindForbidden {
		t.Errorf("outsider: %v", err)
	}
	if _, err := svc.Detail(ctx, advisor, "missing"); kindOf(err) != apperr.KindNotFound {
		t.Errorf("missing project: %v", err)
	}
}

func TestMyPFC(t *testing.T) {
	w := newWorld()
	w.programs = map[string]string{"turma-a": "prog-a"}
	svc, _ := w.service(t)
	ctx := context.Background()

	mine, err := svc.MyPFC(ctx, studentActor("111"))
	if err != nil || mine.Group == nil || mine.Group.Project.ID != "p1" || mine.ProgramID != "prog-a" || mine.Cohort == nil {
		t.Fatalf("member: %+v, %v", mine, err)
	}
	free, err := svc.MyPFC(ctx, studentActor("444"))
	if err != nil || free.Group != nil || free.Cohort == nil || free.Cohort.ID != "turma-a" || free.ProgramID != "prog-a" {
		t.Fatalf("free student: %+v, %v", free, err)
	}
	noProgram, err := svc.MyPFC(ctx, studentActor("333"))
	if err != nil || noProgram.Cohort == nil || noProgram.ProgramID != "" {
		t.Fatalf("cohort without program: %+v, %v", noProgram, err)
	}
	outside, err := svc.MyPFC(ctx, studentActor("000"))
	if err != nil || outside.Cohort != nil || outside.Group != nil {
		t.Fatalf("not pre-authorized: %+v, %v", outside, err)
	}
}

func TestCancelInvitation(t *testing.T) {
	w := newWorld()
	svc, _ := w.service(t)
	ctx := context.Background()

	if err := svc.CancelInvitation(ctx, studentActor("222"), "p1", "222"); kindOf(err) != apperr.KindForbidden || !w.invitations["p1|222"] {
		t.Fatalf("outsider cancelling: %v", err)
	}
	if err := svc.CancelInvitation(ctx, studentActor("111"), "p1", "222"); err != nil || w.invitations["p1|222"] {
		t.Fatalf("member cancelling: %v", err)
	}
	if err := svc.CancelInvitation(ctx, studentActor("111"), "p1", "222"); kindOf(err) != apperr.KindNotFound {
		t.Errorf("cancelling twice: %v", err)
	}
}

func TestAddMemberRequiresPreauthorization(t *testing.T) {
	w := newWorld()
	svc, buf := w.service(t)
	err := svc.AddMember(context.Background(), "p1", MemberInput{RGM: "777"})
	if e, ok := apperr.As(err); !ok || e.Kind != apperr.KindNotFound || e.Message != msgNotPreauthorized {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(buf.String(), fmt.Sprintf(`"motivo":%q`, msgNotPreauthorized)) {
		t.Errorf("audit = %s", buf)
	}
}

func TestMembership(t *testing.T) {
	p := Project{Members: []string{"111"}}
	staff := actor.Actor{Role: actor.RoleAdmin, Email: "111@alunos.umc.br"}
	if !IsMember(studentActor("111"), p) || IsMember(staff, p) || IsMember(actor.Actor{}, p) {
		t.Error("IsMember")
	}
	if !CanView(staff, p) || CanView(studentActor("222"), p) {
		t.Error("CanView")
	}
}

func TestMembersAdvisedBy(t *testing.T) {
	w := newWorld()
	w.projects["p2"] = Project{ID: "p2", ProgramID: "prog-b", Members: []string{"333"}}
	w.projects["p3"] = Project{ID: "p3", ProgramID: "prog-a", Members: []string{"444"}, AdvisorID: "outro"}
	w.advise("p1")
	w.advise("p2")
	svc, _ := w.service(t)
	got, err := svc.MembersAdvisedBy(context.Background(), advisor.Sub)
	slices.Sort(got)
	if err != nil || !slices.Equal(got, []string{"111", "333"}) {
		t.Fatalf("members = %v, err = %v", got, err)
	}
	if got, _ := svc.MembersAdvisedBy(context.Background(), ""); len(got) != 0 {
		t.Errorf("no sub: %v", got)
	}
}
