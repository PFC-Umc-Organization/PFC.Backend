package usuario

import (
	"context"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/academic"
)

// UseAcademic wires the academic module in; the identity module replaces
// this package in phase 6.
func UseAcademic(a *academic.Service) { turmaDoRGM = a.CohortOf }

// Accounts adapts the account lookups to groups.Accounts until phase 6.
type Accounts struct{}

func (Accounts) StudentExists(ctx context.Context, rgm string) (bool, error) {
	return AlunoExiste(ctx, rgm)
}

func (Accounts) AdvisorExists(ctx context.Context, sub string) (bool, error) {
	return OrientadorExiste(ctx, sub)
}
