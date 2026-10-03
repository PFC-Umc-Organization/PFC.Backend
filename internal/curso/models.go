package curso

// cursosDisponiveis é a lista fechada de cursos que o sistema cobre por
// enquanto — só a área de TI da faculdade. Mesma lista que
// `CURSOS_DISPONIVEIS` no frontend (curso.model.ts); validado aqui também
// pra não depender só da UI (alguém podendo chamar a API direto).
var cursosDisponiveis = map[string]bool{
	"Engenharia de Software": true,
	"Sistemas de Informação": true,
}

func nomeValido(nome string) bool {
	return cursosDisponiveis[nome]
}

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
