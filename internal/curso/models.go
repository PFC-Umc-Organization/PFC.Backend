package curso

// Curso é a Turma em que o PFC é ofertado — espelha `Curso` do frontend
// (curso.model.ts).
type Curso struct {
	ID      string `json:"id"`
	Nome    string `json:"nome"`
	Turno   string `json:"turno"`
	Periodo string `json:"periodo"`
}

// NovoCurso é o corpo esperado em POST /turmas.
type NovoCurso struct {
	Nome    string `json:"nome"`
	Turno   string `json:"turno"`
	Periodo string `json:"periodo"`
}

// AtualizarCurso é o corpo esperado em PUT /turmas/:turmaId.
type AtualizarCurso struct {
	Nome    string `json:"nome"`
	Turno   string `json:"turno"`
	Periodo string `json:"periodo"`
}
