package referencia

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
)

var buscarProjeto func(context.Context, string) (groups.Project, bool, error)

// UseGroups wires the groups module in until this package becomes a module.
func UseGroups(g *groups.Service) { buscarProjeto = g.Project }
