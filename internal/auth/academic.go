package auth

import "github.com/PFC-Umc-Organization/PFC.Backend/internal/academic"

// UseAcademic wires the academic module in; the identity module replaces
// this package in phase 6.
func UseAcademic(a *academic.Service) { turmaDoRGM = a.CohortOf }
