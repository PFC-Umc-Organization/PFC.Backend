package matricula

// RGMRequest é o corpo esperado em POST/DELETE /admin/students.
type RGMRequest struct {
	RGMs []string `json:"rgms"`
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
	RGM    string `json:"rgm"`
	Status string `json:"status"`
}
