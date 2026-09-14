package programa


type Programa struct {
	ID      string `json:"id"`
	CursoID string `json:"cursoId"`
}


type NovoPrograma struct {
	CursoID string `json:"cursoId"`
}

type AtualizarPrograma struct {
	CursoID string `json:"cursoId"`
}