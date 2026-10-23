package groups

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/audit"
)

type Service struct {
	store    Store
	academic Academic
	accounts Accounts
	newID    func() string
	now      func() time.Time
	// notify is the e-mail hook for invitations. There is no e-mail
	// provider yet: students answer invitations in the app.
	notify func(context.Context, Invitation)
	// deleted runs after a project is gone, so other modules clean up what
	// they keep for it.
	deleted []func(ctx context.Context, projectID string)
}

func NewService(store Store, academic Academic, accounts Accounts) *Service {
	return &Service{
		store:    store,
		academic: academic,
		accounts: accounts,
		newID:    uuid.NewString,
		now:      time.Now,
		notify: func(ctx context.Context, inv Invitation) {
			slog.InfoContext(ctx, "invitation created", "rgm", inv.RGM, "project", inv.ProjectID, "invitedBy", inv.InvitedBy)
		},
	}
}

func projectResource(id string) *audit.Resource { return &audit.Resource{Type: "projeto", ID: id} }

func failure(ctx context.Context, action, reason string, resource *audit.Resource, details map[string]any) {
	audit.Record(ctx, audit.Event{Action: action, Result: audit.Failure, Reason: reason, Resource: resource, Details: details})
}

func success(ctx context.Context, action string, resource *audit.Resource, details map[string]any) {
	audit.Record(ctx, audit.Event{Action: action, Result: audit.Success, Resource: resource, Details: details})
}

func writeError(err error) error {
	if errors.Is(err, errNotFound) {
		return apperr.NotFound(msgProjectNotFound)
	}
	return apperr.Internal("falha ao salvar — tente novamente em instantes", err)
}

func (s *Service) Project(ctx context.Context, id string) (Project, bool, error) {
	return s.store.GetProject(ctx, id)
}

func (s *Service) load(ctx context.Context, id string) (Project, error) {
	p, found, err := s.store.GetProject(ctx, id)
	if err != nil {
		return Project{}, apperr.Internal("falha ao carregar projeto", err)
	}
	if !found {
		return Project{}, apperr.NotFound(msgProjectNotFound)
	}
	return p, nil
}

// CreateProject is the academic team's path; members join afterwards
// through AddMember, which validates each RGM.
func (s *Service) CreateProject(ctx context.Context, programID string, in ProjectInput) (Project, error) {
	exists, err := s.academic.ProgramExists(ctx, programID)
	if err != nil {
		return Project{}, apperr.Internal("falha ao validar programa", err)
	}
	if !exists {
		return Project{}, apperr.NotFound(msgProgramNotFound)
	}
	in = in.normalized()
	if msg := in.validate(); msg != "" {
		return Project{}, apperr.Invalid(msg)
	}
	p := Project{ID: s.newID(), Name: in.Name, Description: in.Description, ProgramID: programID, Members: []string{}}
	if err := s.store.CreateProject(ctx, p); err != nil {
		return Project{}, apperr.Internal("falha ao criar projeto", err)
	}
	success(ctx, "projeto.criado", projectResource(p.ID), map[string]any{"programaId": programID, "nome": p.Name})
	return p, nil
}

// ProjectsOfProgram gives an advisor only the projects they advise.
func (s *Service) ProjectsOfProgram(ctx context.Context, viewer actor.Actor, programID string) ([]Project, error) {
	projects, err := s.store.ProjectsOfProgram(ctx, programID)
	if err != nil {
		return nil, apperr.Internal("falha ao listar projetos", err)
	}
	if viewer.HasRole(actor.RoleAdmin) {
		return projects, nil
	}
	return slices.DeleteFunc(projects, func(p Project) bool { return !Advises(viewer, p) }), nil
}

func (s *Service) Detail(ctx context.Context, viewer actor.Actor, id string) (Detail, error) {
	p, err := s.load(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	if !CanView(viewer, p) {
		return Detail{}, apperr.Forbidden(msgMembersAndStaffOnly)
	}
	return s.detail(ctx, p), nil
}

// detail fills in names and cohort. They only decorate the project: a failed
// lookup is logged and leaves the field empty instead of failing the page.
func (s *Service) detail(ctx context.Context, p Project) Detail {
	d := Detail{Project: p, Members: make([]Member, 0, len(p.Members))}
	var names map[string]string
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		var err error
		if names, err = s.accounts.StudentNames(ctx, p.Members); err != nil {
			slog.WarnContext(ctx, "member names not loaded", "project", p.ID, "error", err)
		}
	}()
	go func() {
		defer wg.Done()
		if p.AdvisorID == "" {
			return
		}
		d.Advisor = &Advisor{ID: p.AdvisorID}
		name, _, err := s.accounts.AdvisorName(ctx, p.AdvisorID)
		if err != nil {
			slog.WarnContext(ctx, "advisor name not loaded", "project", p.ID, "error", err)
		}
		d.Advisor.Name = name
	}()
	go func() {
		defer wg.Done()
		var err error
		if d.Cohort, err = s.cohortOfProgram(ctx, p.ProgramID); err != nil {
			slog.WarnContext(ctx, "cohort not loaded", "project", p.ID, "error", err)
		}
	}()
	wg.Wait()
	for _, rgm := range p.Members {
		m := Member{RGM: rgm}
		if name, ok := names[rgm]; ok {
			m.Name = &name
		}
		d.Members = append(d.Members, m)
	}
	return d
}

func (s *Service) cohortOfProgram(ctx context.Context, programID string) (*Cohort, error) {
	cohortID, found, err := s.academic.CohortOfProgram(ctx, programID)
	if err != nil || !found {
		return nil, err
	}
	return s.cohort(ctx, cohortID)
}

func (s *Service) cohort(ctx context.Context, id string) (*Cohort, error) {
	c, found, err := s.academic.Cohort(ctx, id)
	if err != nil || !found {
		return nil, err
	}
	return &c, nil
}

// MyPFC reads the cohort from the allowlist, not from the session: a student
// pre-authorized after signing in sees it without signing in again.
func (s *Service) MyPFC(ctx context.Context, student actor.Actor) (MyPFC, error) {
	rgm := student.RGM()
	p, found, err := s.store.ProjectOfMember(ctx, rgm)
	if err != nil {
		return MyPFC{}, apperr.Internal("falha ao carregar o seu projeto", err)
	}
	if found {
		d := s.detail(ctx, p)
		return MyPFC{Cohort: d.Cohort, ProgramID: p.ProgramID, Group: &d}, nil
	}
	cohortID, err := s.academic.CohortOf(ctx, rgm)
	if err != nil {
		return MyPFC{}, apperr.Internal("falha ao validar a sua matrícula", err)
	}
	if cohortID == "" {
		return MyPFC{}, nil
	}
	cohort, err := s.cohort(ctx, cohortID)
	if err != nil {
		return MyPFC{}, apperr.Internal("falha ao carregar a sua turma", err)
	}
	programID, _, err := s.academic.ProgramOfCohort(ctx, cohortID)
	if err != nil {
		return MyPFC{}, apperr.Internal("falha ao carregar a sua turma", err)
	}
	return MyPFC{Cohort: cohort, ProgramID: programID}, nil
}

func (s *Service) UpdateProject(ctx context.Context, id string, in ProjectInput) error {
	in = in.normalized()
	if msg := in.validate(); msg != "" {
		return apperr.Invalid(msg)
	}
	if err := s.store.UpdateDetails(ctx, id, in.Name, in.Description); err != nil {
		return writeError(err)
	}
	success(ctx, "projeto.atualizado", projectResource(id), map[string]any{"nome": in.Name})
	return nil
}

func (s *Service) DeleteProject(ctx context.Context, id string) error {
	switch err := s.store.DeleteProject(ctx, id); {
	case errors.Is(err, errNotFound):
		return apperr.NotFound(msgProjectNotFound)
	case errors.Is(err, errStale):
		return apperr.Conflict(msgGroupChanged)
	case err != nil:
		return apperr.Internal("falha ao remover projeto", err)
	}
	success(ctx, "projeto.removido", projectResource(id), nil)
	for _, f := range s.deleted {
		f(ctx, id)
	}
	return nil
}

func (s *Service) OnProjectDeleted(f func(ctx context.Context, projectID string)) {
	s.deleted = append(s.deleted, f)
}

func (s *Service) AssignAdvisor(ctx context.Context, id string, in AdvisorInput) error {
	if in.AdvisorID == "" {
		return apperr.Invalid(msgAdvisorRequired)
	}
	exists, err := s.accounts.AdvisorExists(ctx, in.AdvisorID)
	if err != nil {
		return apperr.Internal("falha ao validar orientador", err)
	}
	if !exists {
		failure(ctx, "projeto.orientador_associado", "orientadorId não corresponde a uma conta de orientador", projectResource(id), nil)
		return apperr.NotFound(msgAdvisorNotFound)
	}
	if err := s.store.SetAdvisor(ctx, id, in.AdvisorID); err != nil {
		return writeError(err)
	}
	success(ctx, "projeto.orientador_associado", projectResource(id), map[string]any{"orientadorId": in.AdvisorID})
	return nil
}

func (s *Service) RemoveAdvisor(ctx context.Context, id string) error {
	if err := s.store.ClearAdvisor(ctx, id); err != nil {
		return writeError(err)
	}
	success(ctx, "projeto.orientador_removido", projectResource(id), nil)
	return nil
}

func (s *Service) AddMember(ctx context.Context, id string, in MemberInput) error {
	const action = "projeto.integrante_adicionado"
	rgm := strings.TrimSpace(in.RGM)
	if rgm == "" {
		return apperr.Invalid(msgRGMRequired)
	}
	p, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	if slices.Contains(p.Members, rgm) {
		return apperr.Conflict(msgAlreadyMember(rgm))
	}
	if len(p.Members) >= MaxMembers {
		return apperr.Conflict(msgGroupFull)
	}

	enrolled, err := s.academic.IsEnrolled(ctx, rgm)
	if err != nil {
		return apperr.Internal("falha ao validar matrícula", err)
	}
	// An RGM outside the allowlist may still own a student account (created
	// before the list, or removed from it later): that is a real student.
	if !enrolled {
		hasAccount, err := s.accounts.StudentExists(ctx, rgm)
		if err != nil {
			return apperr.Internal("falha ao validar matrícula", err)
		}
		if !hasAccount {
			failure(ctx, action, msgNotPreauthorized, projectResource(id), map[string]any{"rgm": rgm})
			return apperr.NotFound(msgNotPreauthorized)
		}
	}

	other, taken, err := s.store.ProjectOfMember(ctx, rgm)
	if err != nil {
		return apperr.Internal("falha ao validar o integrante", err)
	}
	if taken {
		failure(ctx, action, "RGM já é integrante de outro projeto", projectResource(id), map[string]any{"rgm": rgm, "outroProjetoId": other.ID})
		return apperr.Conflict(msgInNamedProject(rgm, other.Name))
	}

	switch err := s.store.AddMember(ctx, id, rgm, p.Members); {
	case errors.Is(err, errMemberTaken):
		return apperr.Conflict(msgInOtherProject(rgm))
	case errors.Is(err, errStale):
		return apperr.Conflict(msgGroupChanged)
	case err != nil:
		return apperr.Internal("falha ao adicionar integrante", err)
	}
	success(ctx, action, projectResource(id), map[string]any{"rgm": rgm})
	return nil
}

func (s *Service) RemoveMember(ctx context.Context, id string, in MemberInput) error {
	rgm := strings.TrimSpace(in.RGM)
	if rgm == "" {
		return apperr.Invalid(msgRGMRequired)
	}
	switch err := s.store.RemoveMember(ctx, id, rgm); {
	case errors.Is(err, errNotFound):
		return apperr.NotFound(msgProjectNotFound)
	case errors.Is(err, errNotMember):
		return apperr.NotFound(msgNotMember(rgm))
	case errors.Is(err, errStale):
		return apperr.Conflict(msgGroupChanged)
	case err != nil:
		return apperr.Internal("falha ao remover integrante", err)
	}
	success(ctx, "projeto.integrante_removido", projectResource(id), map[string]any{"rgm": rgm})
	return nil
}

// CreateOwnProject is POST /meu-pfc: a pre-authorized student creates the
// group's project in their cohort's program. Only the creator joins now;
// classmates get invitations and join when they accept.
func (s *Service) CreateOwnProject(ctx context.Context, student actor.Actor, in ProjectInput) (Project, error) {
	fail := func(reason string) { failure(ctx, "projeto.criado_pelo_aluno", reason, nil, nil) }
	rgm := student.RGM()

	in = in.normalized()
	if msg := in.validate(); msg != "" {
		return Project{}, apperr.Invalid(msg)
	}
	members := group(rgm, in.Members)
	if len(members) > MaxMembers {
		return Project{}, apperr.Invalid(msgGroupTooLarge)
	}

	cohorts, err := s.academic.CohortsOf(ctx, members)
	if err != nil {
		return Project{}, apperr.Internal("falha ao validar a sua matrícula", err)
	}
	cohort := cohorts[rgm]
	if cohort == "" {
		fail("RGM sem turma vinculada")
		return Project{}, apperr.Forbidden(msgNoCohort)
	}
	programID, hasProgram, err := s.academic.ProgramOfCohort(ctx, cohort)
	if err != nil {
		return Project{}, apperr.Internal("falha ao validar a turma", err)
	}
	if !hasProgram {
		fail("turma sem programa de PFC")
		return Project{}, apperr.Conflict(msgCohortWithoutProgram)
	}
	taken, err := s.store.ProjectsOfMembers(ctx, members)
	if err != nil {
		return Project{}, apperr.Internal("falha ao validar os integrantes", err)
	}
	for _, r := range members {
		if cohorts[r] != cohort {
			fail("integrante fora da turma")
			return Project{}, apperr.Invalid(msgNotInCohort(r))
		}
		if _, ok := taken[r]; ok {
			fail("integrante já está em um projeto")
			if r == rgm {
				return Project{}, apperr.Conflict(msgAlreadyInProject)
			}
			return Project{}, apperr.Conflict(msgInOtherProject(r))
		}
	}

	p := Project{ID: s.newID(), Name: in.Name, Description: in.Description, ProgramID: programID, Members: []string{rgm}}
	err = s.store.CreateProject(ctx, p)
	if errors.Is(err, errMemberTaken) {
		fail("integrante já está em um projeto")
		return Project{}, apperr.Conflict(msgAlreadyInProject)
	}
	if err != nil {
		return Project{}, apperr.Internal("falha ao criar projeto", err)
	}

	// A failed invitation does not undo the project: the group can invite
	// again through POST /projetos/:projetoId/convites.
	invited := []string{}
	for _, classmate := range members[1:] {
		inv, err := s.invite(ctx, p, classmate, student.Name)
		if err != nil {
			slog.WarnContext(ctx, "invitation not created", "project", p.ID, "rgm", classmate, "error", err)
			continue
		}
		s.notify(ctx, inv)
		invited = append(invited, classmate)
	}

	success(ctx, "projeto.criado_pelo_aluno", projectResource(p.ID), map[string]any{"programaId": programID, "nome": p.Name, "convidados": invited})
	return p, nil
}

func (s *Service) invite(ctx context.Context, p Project, rgm, invitedBy string) (Invitation, error) {
	inv := Invitation{
		ProjectID:   p.ID,
		ProjectName: p.Name,
		RGM:         rgm,
		InvitedBy:   invitedBy,
		CreatedAt:   s.now().UTC().Format(time.RFC3339),
	}
	return inv, s.store.CreateInvitation(ctx, inv)
}

func (s *Service) InvitationsOf(ctx context.Context, rgm string) ([]Invitation, error) {
	invitations, err := s.store.InvitationsOfStudent(ctx, rgm)
	if err != nil {
		return nil, apperr.Internal("falha ao listar convites", err)
	}
	return invitations, nil
}

// AcceptInvitation re-checks the /meu-pfc rules, since they may have
// changed since the invitation: the student must be free and in the
// group's cohort.
func (s *Service) AcceptInvitation(ctx context.Context, rgm, projectID string) (Project, error) {
	fail := func(reason string) { failure(ctx, "convite.aceito", reason, projectResource(projectID), nil) }

	exists, err := s.store.InvitationExists(ctx, projectID, rgm)
	if err != nil {
		return Project{}, apperr.Internal("falha ao carregar convite", err)
	}
	if !exists {
		return Project{}, apperr.NotFound(msgInvitationNotFound)
	}
	p, found, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return Project{}, apperr.Internal("falha ao carregar o grupo", err)
	}
	if !found {
		if err := s.store.DeleteInvitation(ctx, projectID, rgm); err != nil {
			slog.WarnContext(ctx, "orphan invitation not deleted", "project", projectID, "rgm", rgm, "error", err)
		}
		fail("grupo não existe mais")
		return Project{}, apperr.NotFound(msgGroupGone)
	}

	_, taken, err := s.store.ProjectOfMember(ctx, rgm)
	if err != nil {
		return Project{}, apperr.Internal("falha ao validar a sua participação", err)
	}
	if taken {
		fail("aluno já está em um projeto")
		return Project{}, apperr.Conflict(msgAlreadyInProject)
	}
	cohort, err := s.academic.CohortOf(ctx, rgm)
	if err != nil {
		return Project{}, apperr.Internal("falha ao validar a sua matrícula", err)
	}
	programID, hasProgram, err := s.academic.ProgramOfCohort(ctx, cohort)
	if err != nil {
		return Project{}, apperr.Internal("falha ao validar a sua turma", err)
	}
	if cohort == "" || !hasProgram || programID != p.ProgramID {
		fail("aluno fora da turma do grupo")
		return Project{}, apperr.Conflict(msgOutsideGroupCohort)
	}

	switch err := s.store.AddMember(ctx, projectID, rgm, p.Members); {
	case errors.Is(err, errMemberTaken):
		fail("aluno já está em um projeto")
		return Project{}, apperr.Conflict(msgAlreadyInProject)
	case errors.Is(err, errStale):
		fail("grupo cheio ou aluno já é integrante")
		return Project{}, apperr.Conflict(msgCannotJoin)
	case err != nil:
		return Project{}, apperr.Internal("falha ao entrar no grupo", err)
	}

	// One project per student: the other invitations lost their purpose.
	// Failing to clean them up does not undo the join.
	pending, err := s.store.InvitationsOfStudent(ctx, rgm)
	if err != nil {
		slog.WarnContext(ctx, "invitations not cleaned up", "rgm", rgm, "error", err)
	}
	for _, inv := range pending {
		if err := s.store.DeleteInvitation(ctx, inv.ProjectID, rgm); err != nil && !errors.Is(err, errNotFound) {
			slog.WarnContext(ctx, "invitation not cleaned up", "project", inv.ProjectID, "rgm", rgm, "error", err)
		}
	}

	success(ctx, "convite.aceito", projectResource(projectID), map[string]any{"rgm": rgm})
	p.Members = append(p.Members, rgm)
	return p, nil
}

func (s *Service) DeclineInvitation(ctx context.Context, rgm, projectID string) error {
	switch err := s.store.DeleteInvitation(ctx, projectID, rgm); {
	case errors.Is(err, errNotFound):
		return apperr.NotFound(msgInvitationNotFound)
	case err != nil:
		return apperr.Internal("falha ao recusar convite", err)
	}
	success(ctx, "convite.recusado", projectResource(projectID), map[string]any{"rgm": rgm})
	return nil
}

func (s *Service) InvitationsOfProject(ctx context.Context, viewer actor.Actor, projectID string) ([]Invitation, error) {
	p, err := s.load(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if !CanView(viewer, p) {
		return nil, apperr.Forbidden(msgMembersAndStaffOnly)
	}
	invitations, err := s.store.InvitationsOfProject(ctx, p.ID)
	if err != nil {
		return nil, apperr.Internal("falha ao listar convites", err)
	}
	return invitations, nil
}

// Invite follows the /meu-pfc rules; pending invitations count towards the
// group limit.
func (s *Service) Invite(ctx context.Context, member actor.Actor, projectID string, in MemberInput) (Invitation, error) {
	p, err := s.load(ctx, projectID)
	if err != nil {
		return Invitation{}, err
	}
	fail := func(reason string) { failure(ctx, "convite.enviado", reason, projectResource(p.ID), nil) }
	if !IsMember(member, p) {
		fail("apenas integrantes podem convidar")
		return Invitation{}, apperr.Forbidden(msgMembersOnlyInvite)
	}
	classmate := strings.TrimSpace(in.RGM)
	if classmate == "" {
		return Invitation{}, apperr.Invalid(msgRGMRequired)
	}
	if slices.Contains(p.Members, classmate) {
		return Invitation{}, apperr.Conflict(msgAlreadyMember(classmate))
	}

	cohort, err := s.academic.CohortOf(ctx, classmate)
	if err != nil {
		return Invitation{}, apperr.Internal("falha ao validar o convidado", err)
	}
	programID, hasProgram, err := s.academic.ProgramOfCohort(ctx, cohort)
	if err != nil {
		return Invitation{}, apperr.Internal("falha ao validar o convidado", err)
	}
	if cohort == "" || !hasProgram || programID != p.ProgramID {
		fail("convidado fora da turma do grupo")
		return Invitation{}, apperr.Invalid(msgNotInGroupCohort(classmate))
	}
	_, taken, err := s.store.ProjectOfMember(ctx, classmate)
	if err != nil {
		return Invitation{}, apperr.Internal("falha ao validar o convidado", err)
	}
	if taken {
		fail("convidado já está em um projeto")
		return Invitation{}, apperr.Conflict(msgInOtherProject(classmate))
	}
	pending, err := s.store.InvitationsOfProject(ctx, p.ID)
	if err != nil {
		return Invitation{}, apperr.Internal("falha ao validar o tamanho do grupo", err)
	}
	if len(p.Members)+len(pending) >= MaxMembers {
		fail("grupo cheio")
		return Invitation{}, apperr.Conflict(msgGroupFullWithPending)
	}

	inv, err := s.invite(ctx, p, classmate, member.Name)
	if errors.Is(err, errDuplicate) {
		return Invitation{}, apperr.Conflict(msgAlreadyInvited(classmate))
	}
	if err != nil {
		return Invitation{}, apperr.Internal("falha ao enviar convite", err)
	}
	s.notify(ctx, inv)
	success(ctx, "convite.enviado", projectResource(p.ID), map[string]any{"rgm": classmate})
	return inv, nil
}

func (s *Service) CancelInvitation(ctx context.Context, member actor.Actor, projectID, rgm string) error {
	p, err := s.load(ctx, projectID)
	if err != nil {
		return err
	}
	if !IsMember(member, p) {
		failure(ctx, "convite.cancelado", "apenas integrantes podem cancelar", projectResource(p.ID), nil)
		return apperr.Forbidden(msgMembersOnlyCancel)
	}
	switch err := s.store.DeleteInvitation(ctx, p.ID, rgm); {
	case errors.Is(err, errNotFound):
		return apperr.NotFound(msgInvitationNotFound)
	case err != nil:
		return apperr.Internal("falha ao cancelar convite", err)
	}
	success(ctx, "convite.cancelado", projectResource(p.ID), map[string]any{"rgm": rgm})
	return nil
}
