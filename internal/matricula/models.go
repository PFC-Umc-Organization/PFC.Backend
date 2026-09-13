package matricula

// RGMRequest é o corpo esperado em POST/DELETE /admin/students.
type RGMRequest struct {
	RGMs []string `json:"rgms"`
}

// RGMFalha registra um RGM que não pôde ser processado, com o motivo —
// permite que o restante do lote siga em frente mesmo se um item falhar
// (mesmo comportamento que o harness já validava manualmente).
type RGMFalha struct {
	RGM  string `json:"rgm"`
	Erro string `json:"erro"`
}

// RGMResponse é a resposta de sucesso parcial/total do processamento em lote.
type RGMResponse struct {
	Processados int        `json:"processados"`
	Falhas      []RGMFalha `json:"falhas,omitempty"`
}

// Matricula é um item da allowlist, devolvido por GET /admin/students.
// Espelha `Matricula` do frontend (`rgm` + `status`).
type Matricula struct {
	RGM    string `json:"rgm"`
	Status string `json:"status"`
}
