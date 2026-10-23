package integracao

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

type ticket struct {
	URL    string            `json:"url"`
	Campos map[string]string `json:"campos"`
	Chave  string            `json:"chave"`
}

// enviar faz o que o navegador faz: o formulário assinado direto para o S3.
func enviar(t *testing.T, tk ticket, conteudo string) int {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range tk.Campos {
		w.WriteField(k, v)
	}
	part, _ := w.CreateFormFile("file", "arquivo")
	part.Write([]byte(conteudo))
	w.Close()
	resp, err := http.Post(tk.URL, w.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func pedirEnvio(t *testing.T, projeto, atividade, campo, nome string) ticket {
	t.Helper()
	var tk ticket
	body := esperar(t, aluno("111"), "POST", "/projetos/"+projeto+"/entregas/"+atividade+"/arquivos",
		`{"campoId":"`+campo+`","nome":"`+nome+`","tamanho":8}`, 201)
	if err := json.Unmarshal([]byte(body), &tk); err != nil {
		t.Fatal(err)
	}
	return tk
}

func TestArquivos(t *testing.T) {
	resetar(t)
	criarCursos(t, "Engenharia de Software")
	turma := id(t, esperar(t, admin, "POST", "/turmas", `{"nome":"Engenharia de Software","turno":"NOITE","periodo":"2026.2"}`, 201))
	esperar(t, admin, "POST", "/programas", `{"cursoId":"`+turma+`"}`, 201)
	esperar(t, admin, "POST", "/admin/students", `{"turmaId":"`+turma+`","rgms":["111","222"]}`, 200)
	p1 := id(t, esperar(t, aluno("111"), "POST", "/meu-pfc", `{"nome":"Athena"}`, 201))
	atividade := esperar(t, admin, "POST", "/atividades", `{"titulo":"Monografia","prazo":"2099-12-01T23:59"}`, 201)
	a1, campo := id(t, atividade), jsonField(t, atividade, "campos", 0, "id")
	rota := "/projetos/" + p1 + "/entregas/" + a1

	esperar(t, aluno("111"), "POST", rota+"/arquivos", `{"campoId":"`+campo+`","nome":"virus.exe","tamanho":8}`, 400)
	esperar(t, aluno("111"), "POST", rota+"/arquivos", `{"campoId":"`+campo+`","nome":"a.pdf","tamanho":52428801}`, 400)
	esperar(t, aluno("222"), "POST", rota+"/arquivos", `{"campoId":"`+campo+`","nome":"a.pdf","tamanho":8}`, 403)

	tk := pedirEnvio(t, p1, a1, campo, "Monografia Final.pdf")
	// Sem o upload, a chave ainda não vale como resposta.
	esperar(t, aluno("111"), "PUT", rota, `{"respostas":{"`+campo+`":"`+tk.Chave+`"}}`, 400)
	esperar(t, aluno("111"), "PUT", rota, `{"respostas":{"`+campo+`":"monografia.pdf"}}`, 400)

	adulterado := tk
	adulterado.Campos = map[string]string{}
	for k, v := range tk.Campos {
		adulterado.Campos[k] = v
	}
	adulterado.Campos["Content-Type"] = "text/html"
	if status := enviar(t, adulterado, "<script>"); status != http.StatusForbidden {
		t.Fatalf("Content-Type trocado: S3 respondeu %d", status)
	}
	if status := enviar(t, tk, "conteudo"); status != http.StatusNoContent {
		t.Fatalf("upload: S3 respondeu %d", status)
	}
	esperar(t, aluno("111"), "PUT", rota, `{"respostas":{"`+campo+`":"`+tk.Chave+`"}}`, 200)

	var d struct{ URL, Nome string }
	json.Unmarshal([]byte(esperar(t, admin, "GET", rota+"/arquivos/"+campo, "", 200)), &d)
	resp, err := http.Get(d.URL)
	if err != nil {
		t.Fatal(err)
	}
	conteudo, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(conteudo) != "conteudo" || d.Nome != "Monografia Final.pdf" ||
		!strings.Contains(resp.Header.Get("Content-Disposition"), "Monografia%20Final.pdf") {
		t.Fatalf("download: %q %q %q", conteudo, d.Nome, resp.Header.Get("Content-Disposition"))
	}
	esperar(t, aluno("222"), "GET", rota+"/arquivos/"+campo, "", 403)

	// Trocar o arquivo apaga o anterior; devolver a entrega apaga o atual.
	novo := pedirEnvio(t, p1, a1, campo, "v2.docx")
	enviar(t, novo, "versao 2")
	esperar(t, aluno("111"), "PUT", rota, `{"respostas":{"`+campo+`":"`+novo.Chave+`"}}`, 200)
	if bucket.has(tk.Chave) || !bucket.has(novo.Chave) {
		t.Fatal("o arquivo substituído devia ter sido apagado")
	}
	esperar(t, admin, "DELETE", rota, "", 200)
	if bucket.has(novo.Chave) {
		t.Fatal("o arquivo da entrega devolvida devia ter sido apagado")
	}
	esperar(t, admin, "GET", rota+"/arquivos/"+campo, "", 404)

	// Prazo vencido sem entrega: nem arquivo nem envio.
	vencida := esperar(t, admin, "POST", "/atividades", `{"titulo":"Banca","prazo":"2020-01-01T23:59"}`, 201)
	rotaVencida := "/projetos/" + p1 + "/entregas/" + id(t, vencida)
	campoVencido := jsonField(t, vencida, "campos", 0, "id")
	esperar(t, aluno("111"), "POST", rotaVencida+"/arquivos", `{"campoId":"`+campoVencido+`","nome":"a.pdf","tamanho":8}`, 409)
	esperar(t, aluno("111"), "PUT", rotaVencida, `{"respostas":{"`+campoVencido+`":"a.pdf"}}`, 409)

	// Material em arquivo, só para a turma: o aluno de outra turma não vê.
	var mt ticket
	json.Unmarshal([]byte(esperar(t, admin, "POST", "/materiais/arquivos", `{"nome":"Modelo PFC.docx","tamanho":5}`, 201)), &mt)
	esperar(t, admin, "POST", "/materiais", `{"titulo":"Modelo","tipo":"MODELO","arquivo":"`+mt.Chave+`"}`, 400)
	if status := enviar(t, mt, "docx!"); status != http.StatusNoContent {
		t.Fatalf("upload do material: S3 respondeu %d", status)
	}
	outra := id(t, esperar(t, admin, "POST", "/turmas", `{"nome":"Engenharia de Software","turno":"MANHA","periodo":"2026.2"}`, 201))
	esperar(t, admin, "POST", "/admin/students", `{"turmaId":"`+outra+`","rgms":["333"]}`, 200)
	material := id(t, esperar(t, admin, "POST", "/materiais", `{"titulo":"Modelo","tipo":"MODELO","arquivo":"`+mt.Chave+`","cursoId":"`+turma+`"}`, 201))
	esperar(t, aluno("111"), "GET", "/materiais/"+material+"/arquivo", "", 200)
	esperar(t, aluno("333"), "GET", "/materiais/"+material+"/arquivo", "", 404)
	if body := esperar(t, aluno("333"), "GET", "/materiais", "", 200); body != "[]" {
		t.Fatalf("aluno de outra turma vê o material: %s", body)
	}
	esperar(t, admin, "DELETE", "/materiais/"+material, "", 200)
	if bucket.has(mt.Chave) {
		t.Fatal("o arquivo do material removido devia ter sido apagado")
	}
}
