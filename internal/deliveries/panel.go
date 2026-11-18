package deliveries

import (
	"context"
	"errors"
	"fmt"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/apperr"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/parallel"
)

// ---------------------------------------------------------------------------
// Painel
// ---------------------------------------------------------------------------

// Resposta
type Panel struct {
	Projects    []groups.Project `json:"projetos"`
	Activities  []Activity       `json:"atividades"`
	Submissions []Submission     `json:"entregas"`
	Penalties   []Penalties      `json:"penalidades"`
}

// Montagem
func emptyPanel() Panel {
	return Panel{Projects: []groups.Project{}, Activities: []Activity{}, Submissions: []Submission{}, Penalties: []Penalties{}}
}

// Consulta
func (s *Service) Panel(ctx context.Context, viewer actor.Actor, programID string) (Panel, error) {
	const msg = "Não foi possível carregar o painel. Tente novamente em instantes."
	projects, err := s.projects.ProjectsOfProgram(ctx, viewer, programID)
	if err != nil {
		return Panel{}, err
	}
	if len(projects) == 0 {
		return emptyPanel(), nil
	}

	var activities []Activity
	var activitiesErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		activities, activitiesErr = s.activitiesOf(ctx, projects[0])
	}()
	sources, err := parallel.Map(ctx, projects, func(ctx context.Context, p groups.Project) (penaltySources, error) {
		return s.projectPenalties(ctx, p.ID)
	})
	<-done
	if err := errors.Join(activitiesErr, err); err != nil {
		return Panel{}, apperr.Internal(msg, fmt.Errorf("panel of program %s: %w", programID, err))
	}

	out := emptyPanel()
	out.Projects = projects
	out.Activities = activities
	if out.Activities == nil {
		out.Activities = []Activity{}
	}
	now := s.now()
	for i, p := range projects {
		out.Submissions = append(out.Submissions, sources[i].submissions...)
		out.Penalties = append(out.Penalties, penaltiesOf(p.ID, activities, sources[i].submissions, sources[i].records, now))
	}
	return out, nil
}
