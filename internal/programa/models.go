package programa

// Programa agrupa projetos e associações de orientador sob um Curso já
// existente (gerido no frontend/outro lugar — aqui só referenciamos o id,
// sem duplicar nome/turno/período que já vivem no Curso).
type Programa struct {
	ID       string `json:"id"`
	CursoID  string `json:"cursoId"`
}

// NovoPrograma é o corpo esperado em POST /programas.
type NovoPrograma struct {
	CursoID string `json:"cursoId"`
}
