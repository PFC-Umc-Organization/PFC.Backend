package matricula

// RGMRequest é o corpo esperado em POST/DELETE /admin/students. TurmaID só
// é usado no POST (pré-autorizar) — o DELETE ignora o campo, já que remover
// é só por RGM.
type RGMRequest struct {
	RGMs    []string `json:"rgms"`
	TurmaID string   `json:"turmaId"`
}


type RGMFalha struct {
	RGM  string `json:"rgm"`
	Erro string `json:"erro"`
}


type RGMResponse struct {
	Processados int        `json:"processados"`
	Falhas      []RGMFalha `json:"falhas,omitempty"`
}


type Matricula struct {
	RGM     string `json:"rgm"`
	Status  string `json:"status"`
	TurmaID string `json:"turmaId,omitempty"`
}
