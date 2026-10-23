package integracao

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/contrato.golden")

const goldenPath = "testdata/contrato.golden"

var (
	uuidPattern  = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	timePattern  = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z`)
	accessedDate = regexp.MustCompile(`Acesso em: \d{1,2} [a-zç]{3,4}\.? \d{4}`)
)

const fakeSubPrefix = "00000000-0000-4000-8000-"

// transcript records every call so the whole API surface can be diffed.
type transcript struct {
	t   *testing.T
	out bytes.Buffer
	ids map[string]string
}

func (tr *transcript) normalize(s string) string {
	s = uuidPattern.ReplaceAllStringFunc(s, func(id string) string {
		if strings.HasPrefix(id, fakeSubPrefix) {
			return id
		}
		if p, ok := tr.ids[id]; ok {
			return p
		}
		p := fmt.Sprintf("<id%d>", len(tr.ids)+1)
		tr.ids[id] = p
		return p
	})
	s = timePattern.ReplaceAllString(s, "<ts>")
	return accessedDate.ReplaceAllString(s, "Acesso em: <data>")
}

// canonicalJSON sorts object keys and top-level array items, after ids became
// placeholders: raw order depends on random ids (map keys, GSI sort keys).
func canonicalJSON(body string) string {
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return body
	}
	encode := func(x any) string {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.Encode(x)
		return strings.TrimSuffix(buf.String(), "\n")
	}
	if list, ok := v.([]any); ok {
		sort.SliceStable(list, func(a, b int) bool { return encode(list[a]) < encode(list[b]) })
	}
	return encode(v)
}

func (tr *transcript) call(q quem, method, path, body string) (int, string) {
	tr.t.Helper()
	status, resp := chamar(tr.t, q, method, path, body)
	who := q.perfil
	if who == "" {
		who = "público"
	}
	fmt.Fprintf(&tr.out, "%s %s [%s]\n", method, tr.normalize(path), who)
	if body != "" {
		fmt.Fprintf(&tr.out, "> %s\n", tr.normalize(body))
	}
	fmt.Fprintf(&tr.out, "< %d %s\n\n", status, canonicalJSON(tr.normalize(resp)))
	return status, resp
}

func (tr *transcript) expect(q quem, method, path, body string, status int) string {
	tr.t.Helper()
	got, resp := tr.call(q, method, path, body)
	if got != status {
		tr.t.Errorf("%s %s = %d, queria %d — %s", method, path, got, status, resp)
	}
	return resp
}

func jsonField(t *testing.T, body string, path ...any) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("json inválido: %s", body)
	}
	for _, p := range path {
		switch k := p.(type) {
		case string:
			v = v.(map[string]any)[k]
		case int:
			v = v.([]any)[k]
		}
	}
	s, _ := v.(string)
	return s
}

func TestContrato(t *testing.T) {
	resetar(t)
	tr := &transcript{t: t, ids: map[string]string{}}
	pub := quem{}
	prof := quem{"ORIENTADOR", "prof@umc.br"}

	cognito.add("coord@umc.br", "Coordenação", "ADMIN", true)
	profSub := cognito.add("prof@umc.br", "Prof. Orientador", "ORIENTADOR", true)
	cognito.add("111@alunos.umc.br", "Aluno Um", "ALUNO", true)
	cognito.add("222@alunos.umc.br", "Aluno Dois", "ALUNO", true)
	cognito.add("333@alunos.umc.br", "Aluno Três", "", true)
	cognito.add("444@alunos.umc.br", "Aluno Quatro", "ALUNO", false)
	cognito.add("666@alunos.umc.br", "Aluno Sem Matrícula", "ALUNO", true)

	// auth
	tr.expect(pub, "POST", "/auth/login", `{`, 400)
	tr.expect(pub, "POST", "/auth/login", `{"email":"coord@umc.br"}`, 400)
	tr.expect(pub, "POST", "/auth/login", `{"email":"coord@umc.br","senha":"errada"}`, 401)
	tr.expect(pub, "POST", "/auth/login", `{"email":"444@alunos.umc.br","senha":"Senha#123"}`, 403)
	tr.expect(pub, "POST", "/auth/login", `{"email":" COORD@umc.br ","senha":"Senha#123"}`, 200)
	tr.expect(pub, "POST", "/auth/registrar", `{"nome":"x"}`, 400)
	tr.expect(pub, "POST", "/auth/registrar", `{"nome":"Fulano","email":"fulano@gmail.com","senha":"Senha#123"}`, 403)
	tr.expect(pub, "POST", "/auth/registrar", `{"nome":"Fulano","email":"555@alunos.umc.br","senha":"curta"}`, 400)
	tr.expect(pub, "POST", "/auth/registrar", `{"nome":"Aluno Um","email":"111@alunos.umc.br","senha":"Senha#123"}`, 409)
	tr.expect(pub, "POST", "/auth/registrar", `{"nome":"Aluno Cinco","email":"555@alunos.umc.br","senha":"Senha#123","perfil":"ADMIN"}`, 202)
	tr.expect(pub, "POST", "/auth/confirmar", `{"email":"555@alunos.umc.br"}`, 400)
	tr.expect(pub, "POST", "/auth/confirmar", `{"email":"555@alunos.umc.br","codigo":"000000"}`, 400)
	tr.expect(pub, "POST", "/auth/confirmar", `{"email":"nada@alunos.umc.br","codigo":"123456"}`, 404)
	tr.expect(pub, "POST", "/auth/confirmar", `{"email":"555@alunos.umc.br","codigo":"123456"}`, 200)
	tr.expect(pub, "POST", "/auth/confirmar", `{"email":"555@alunos.umc.br","codigo":"123456"}`, 409)
	tr.expect(pub, "POST", "/auth/reenviar-codigo", `{}`, 400)
	tr.expect(pub, "POST", "/auth/reenviar-codigo", `{"email":"nada@alunos.umc.br"}`, 404)
	tr.expect(pub, "POST", "/auth/reenviar-codigo", `{"email":"444@alunos.umc.br"}`, 200)

	// contas
	tr.expect(aluno("111"), "POST", "/admin/usuarios", `{"nome":"X","email":"x@umc.br","perfil":"ORIENTADOR"}`, 403)
	tr.expect(admin, "POST", "/admin/usuarios", `{"nome":"X","email":"x@gmail.com","perfil":"ORIENTADOR"}`, 400)
	tr.expect(admin, "POST", "/admin/usuarios", `{"nome":"X","email":"x@umc.br","perfil":"ALUNO"}`, 400)
	tr.expect(admin, "POST", "/admin/usuarios", `{"nome":"Nova Prof","email":"Nova@UMC.br","perfil":"ORIENTADOR"}`, 201)
	tr.expect(admin, "POST", "/admin/usuarios", `{"nome":"Nova Prof","email":"nova@umc.br","perfil":"ADMIN"}`, 409)

	// primeiro acesso de conta criada pelo admin: a senha temporária pede uma nova
	novaSessao := tr.expect(pub, "POST", "/auth/login", `{"email":"nova@umc.br","senha":"temporaria"}`, 200)
	var desafio struct {
		Sessao string `json:"sessao"`
	}
	if err := json.Unmarshal([]byte(novaSessao), &desafio); err != nil || desafio.Sessao == "" {
		t.Fatalf("login com senha temporária devia devolver o desafio: %s", novaSessao)
	}
	tr.expect(pub, "POST", "/auth/nova-senha", `{"email":"nova@umc.br","senha":"Nova#1234"}`, 400)
	tr.expect(pub, "POST", "/auth/nova-senha", `{"email":"nova@umc.br","sessao":"outra","senha":"Nova#1234"}`, 401)
	tr.expect(pub, "POST", "/auth/nova-senha", `{"email":"nova@umc.br","sessao":"`+desafio.Sessao+`","senha":"curta"}`, 400)
	tr.expect(pub, "POST", "/auth/nova-senha", `{"email":"nova@umc.br","sessao":"`+desafio.Sessao+`","senha":"Nova#1234"}`, 200)
	tr.expect(pub, "POST", "/auth/login", `{"email":"nova@umc.br","senha":"Nova#1234"}`, 200)

	// esqueci a senha: a resposta não diz se a conta existe
	tr.expect(pub, "POST", "/auth/esqueci-senha", `{}`, 400)
	tr.expect(pub, "POST", "/auth/esqueci-senha", `{"email":"ninguem@umc.br"}`, 200)
	tr.expect(pub, "POST", "/auth/esqueci-senha", `{"email":"444@alunos.umc.br"}`, 200)
	tr.expect(pub, "POST", "/auth/esqueci-senha", `{"email":" Prof@UMC.br "}`, 200)
	tr.expect(pub, "POST", "/auth/redefinir-senha", `{"email":"prof@umc.br","codigo":"123456"}`, 400)
	tr.expect(pub, "POST", "/auth/redefinir-senha", `{"email":"prof@umc.br","codigo":"000000","senha":"Outra#1234"}`, 400)
	tr.expect(pub, "POST", "/auth/redefinir-senha", `{"email":"ninguem@umc.br","codigo":"123456","senha":"Outra#1234"}`, 400)
	tr.expect(pub, "POST", "/auth/redefinir-senha", `{"email":"prof@umc.br","codigo":"123456","senha":"curta"}`, 400)
	tr.expect(pub, "POST", "/auth/redefinir-senha", `{"email":"prof@umc.br","codigo":"123456","senha":"Outra#1234"}`, 200)
	tr.expect(pub, "POST", "/auth/login", `{"email":"prof@umc.br","senha":"Outra#1234"}`, 200)

	// quem está logado
	tr.expect(pub, "GET", "/me", "", 401)
	tr.expect(admin, "GET", "/me", "", 200)
	tr.expect(prof, "GET", "/me", "", 200)

	// cursos
	tr.expect(aluno("111"), "POST", "/cursos", `{"nome":"Engenharia de Software"}`, 403)
	tr.expect(admin, "POST", "/cursos", `{"nome":"  "}`, 400)
	tr.expect(admin, "POST", "/cursos", `{"nome":"Engenharia de Software"}`, 201)
	tr.expect(admin, "POST", "/cursos", `{"nome":" engenharia  de software "}`, 409)
	tr.expect(admin, "POST", "/cursos", `{"nome":"Sistemas de Informação"}`, 201)
	direito := id(t, tr.expect(admin, "POST", "/cursos", `{"nome":"Direito"}`, 201))
	tr.expect(aluno("111"), "GET", "/cursos", "", 200)
	tr.expect(admin, "PUT", "/cursos/"+direito, `{"nome":"Engenharia de Software"}`, 409)
	tr.expect(admin, "PUT", "/cursos/inexistente", `{"nome":"Direito Civil"}`, 404)
	tr.expect(aluno("111"), "PUT", "/cursos/"+direito, `{"nome":"Direito Civil"}`, 403)
	tr.expect(admin, "PUT", "/cursos/"+direito, `{"nome":"Direito Civil"}`, 200)
	tr.expect(aluno("111"), "DELETE", "/cursos/"+direito, "", 403)

	// turmas
	tr.expect(aluno("111"), "POST", "/turmas", `{"nome":"Engenharia de Software","turno":"NOITE","periodo":"2026.2"}`, 403)
	tr.expect(admin, "POST", "/turmas", `{"nome":"Engenharia de Software"}`, 400)
	tr.expect(admin, "POST", "/turmas", `{"nome":"Medicina","turno":"NOITE","periodo":"2026.2"}`, 400)
	tr.expect(admin, "POST", "/turmas", `{"nome":"Engenharia de Software","turno":"TARDE","periodo":"2026.2"}`, 400)
	eng := id(t, tr.expect(admin, "POST", "/turmas", `{"nome":"Engenharia de Software","turno":"NOITE","periodo":"2026.2"}`, 201))
	tr.expect(admin, "POST", "/turmas", `{"nome":"Engenharia de Software","turno":"NOITE","periodo":" 2026.2 "}`, 409)
	tr.expect(admin, "POST", "/turmas", `{"nome":"Engenharia de Software","turno":"NOITE","periodo":"2026/2"}`, 409)
	tr.expect(admin, "POST", "/turmas", `{"nome":"Engenharia de Software","turno":"NOITE","periodo":"2026.3"}`, 400)
	tr.expect(admin, "POST", "/turmas", `{"nome":" engenharia  de  software ","turno":"MANHA","periodo":"2026-2"}`, 201)
	si := id(t, tr.expect(admin, "POST", "/turmas", `{"nome":"Sistemas de Informação","turno":"MANHA","periodo":"2026.2"}`, 201))
	extra := id(t, tr.expect(admin, "POST", "/turmas", `{"nome":"Sistemas de Informação","turno":"NOITE","periodo":"2027.1"}`, 201))
	tr.expect(aluno("111"), "GET", "/turmas", "", 200)
	tr.expect(admin, "PUT", "/turmas/"+extra, `{"nome":"Engenharia de Software","turno":"NOITE","periodo":"2026.2"}`, 409)
	tr.expect(admin, "PUT", "/turmas/inexistente", `{"nome":"Engenharia de Software","turno":"NOITE","periodo":"2030.1"}`, 404)
	tr.expect(admin, "PUT", "/turmas/"+extra, `{"nome":"Sistemas de Informação","turno":"NOITE","periodo":"2027.2"}`, 200)
	tr.expect(aluno("111"), "DELETE", "/turmas/"+extra, "", 403)
	tr.expect(admin, "DELETE", "/turmas/"+extra, "", 200)
	tr.expect(admin, "DELETE", "/turmas/"+extra, "", 404)

	// curso com turma não sai; depois que a turma sai, sai
	direitoTurma := id(t, tr.expect(admin, "POST", "/turmas", `{"nome":"Direito Civil","turno":"NOITE","periodo":"2026.2"}`, 201))
	tr.expect(admin, "DELETE", "/cursos/"+direito, "", 409)
	tr.expect(admin, "PUT", "/cursos/"+direito, `{"nome":"Direito Penal"}`, 200)
	tr.expect(admin, "GET", "/turmas", "", 200)
	tr.expect(admin, "DELETE", "/turmas/"+direitoTurma, "", 200)
	tr.expect(admin, "DELETE", "/cursos/"+direito, "", 200)
	tr.expect(admin, "DELETE", "/cursos/"+direito, "", 404)

	// matrículas
	tr.expect(aluno("111"), "POST", "/admin/students", `{"turmaId":"`+eng+`","rgms":["111"]}`, 403)
	tr.expect(admin, "POST", "/admin/students", `{"rgms":["111"]}`, 400)
	tr.expect(admin, "POST", "/admin/students", `{"turmaId":"inexistente","rgms":["111"]}`, 404)
	tr.expect(admin, "POST", "/admin/students", `{"turmaId":"`+eng+`","rgms":[" ",""]}`, 400)
	tr.expect(admin, "POST", "/admin/students", `{"turmaId":"`+eng+`","rgms":["111","222","333","444","777"," 111 ","12a"]}`, 200)
	tr.expect(admin, "POST", "/admin/students", `{"turmaId":"`+si+`","rgms":["888"]}`, 200)
	tr.expect(aluno("111"), "GET", "/admin/students", "", 403)
	tr.expect(admin, "GET", "/admin/students", "", 200)
	tr.expect(admin, "DELETE", "/admin/students", `{"rgms":[]}`, 400)
	tr.expect(admin, "DELETE", "/admin/students", `{"rgms":["777"]}`, 200)
	tr.expect(pub, "POST", "/auth/login", `{"email":"111@alunos.umc.br","senha":"Senha#123"}`, 200)
	tr.expect(admin, "GET", "/usuarios", "", 200)
	tr.expect(aluno("111"), "GET", "/usuarios", "", 403)
	tr.expect(prof, "GET", "/usuarios", "", 200)

	// programas
	tr.expect(aluno("111"), "POST", "/programas", `{"cursoId":"`+eng+`"}`, 403)
	tr.expect(admin, "POST", "/programas", `{}`, 400)
	tr.expect(admin, "POST", "/programas", `{"cursoId":"inexistente"}`, 404)
	prog := id(t, tr.expect(admin, "POST", "/programas", `{"cursoId":"`+eng+`"}`, 201))
	tr.expect(admin, "POST", "/programas", `{"cursoId":"`+eng+`"}`, 409)
	progSI := id(t, tr.expect(admin, "POST", "/programas", `{"cursoId":"`+si+`"}`, 201))
	tr.expect(aluno("111"), "GET", "/programas", "", 200)
	tr.expect(admin, "PUT", "/programas/"+progSI, `{"cursoId":"`+eng+`"}`, 409)
	tr.expect(admin, "PUT", "/programas/inexistente", `{"cursoId":"`+si+`"}`, 404)
	tr.expect(admin, "PUT", "/programas/"+progSI, `{"cursoId":"inexistente"}`, 404)
	tr.expect(admin, "PUT", "/programas/"+progSI, `{"cursoId":"`+si+`"}`, 200)
	tr.expect(admin, "DELETE", "/turmas/"+eng, "", 409)

	// projeto do aluno
	tr.expect(admin, "POST", "/meu-pfc", `{"nome":"X"}`, 403)
	tr.expect(aluno("666"), "POST", "/meu-pfc", `{"nome":"Sem turma"}`, 403)
	tr.expect(aluno("888"), "POST", "/meu-pfc", `{"nome":"Sem programa?"}`, 201)
	tr.expect(aluno("111"), "POST", "/meu-pfc", `{"nome":"  "}`, 400)
	tr.expect(aluno("111"), "POST", "/meu-pfc", `{"nome":"Athena","integrantes":["888"]}`, 400)
	p1 := id(t, tr.expect(aluno("111"), "POST", "/meu-pfc", `{"nome":" Athena ","descricao":"Portal","integrantes":["222"," 222 ",""]}`, 201))
	tr.expect(aluno("111"), "POST", "/meu-pfc", `{"nome":"Outro"}`, 409)
	tr.expect(admin, "GET", "/meu-pfc", "", 403)
	tr.expect(aluno("666"), "GET", "/meu-pfc", "", 200)
	tr.expect(aluno("111"), "GET", "/meu-pfc", "", 200)

	// projetos pela coordenação
	tr.expect(aluno("111"), "POST", "/programas/"+prog+"/projetos", `{"nome":"X"}`, 403)
	tr.expect(admin, "POST", "/programas/inexistente/projetos", `{"nome":"X"}`, 404)
	tr.expect(admin, "POST", "/programas/"+prog+"/projetos", `{"nome":""}`, 400)
	p2 := id(t, tr.expect(admin, "POST", "/programas/"+prog+"/projetos", `{"nome":"Web-Cursos","descricao":"d","integrantes":["999"]}`, 201))
	tr.expect(aluno("111"), "GET", "/programas/"+prog+"/projetos", "", 403)
	tr.expect(admin, "GET", "/programas/"+prog+"/projetos", "", 200)
	tr.expect(prof, "GET", "/programas/"+prog+"/projetos", "", 200)
	tr.expect(aluno("111"), "GET", "/projetos/"+p1, "", 200)
	tr.expect(aluno("111"), "GET", "/projetos/"+p2, "", 403)
	tr.expect(admin, "GET", "/projetos/"+p2, "", 200)
	tr.expect(admin, "GET", "/projetos/inexistente", "", 404)
	tr.expect(aluno("111"), "PUT", "/projetos/"+p2, `{"nome":"Novo"}`, 403)
	tr.expect(admin, "PUT", "/projetos/"+p2, `{"nome":""}`, 400)
	tr.expect(admin, "PUT", "/projetos/inexistente", `{"nome":"Novo"}`, 404)
	tr.expect(admin, "PUT", "/projetos/"+p2, `{"nome":"Web-Cursos 2","descricao":"nova"}`, 200)

	// orientador
	tr.expect(aluno("111"), "PUT", "/projetos/"+p1+"/orientador", `{"orientadorId":"`+profSub+`"}`, 403)
	tr.expect(admin, "PUT", "/projetos/"+p1+"/orientador", `{}`, 400)
	tr.expect(admin, "PUT", "/projetos/"+p1+"/orientador", `{"orientadorId":"`+fakeSubPrefix+`000000000001"}`, 404)
	tr.expect(admin, "PUT", "/projetos/inexistente/orientador", `{"orientadorId":"`+profSub+`"}`, 404)
	tr.expect(admin, "PUT", "/projetos/"+p1+"/orientador", `{"orientadorId":"`+profSub+`"}`, 200)
	tr.expect(admin, "PUT", "/projetos/"+p2+"/orientador", `{"orientadorId":"`+profSub+`"}`, 200)
	tr.expect(aluno("111"), "DELETE", "/projetos/"+p2+"/orientador", "", 403)
	tr.expect(admin, "DELETE", "/projetos/"+p2+"/orientador", "", 200)
	tr.expect(admin, "DELETE", "/projetos/inexistente/orientador", "", 404)
	tr.expect(prof, "GET", "/programas/"+prog+"/projetos", "", 200)
	tr.expect(prof, "GET", "/projetos/"+p1, "", 200)
	tr.expect(prof, "GET", "/projetos/"+p2, "", 403)

	// integrantes pela coordenação
	tr.expect(aluno("111"), "PUT", "/projetos/"+p2+"/integrantes", `{"rgm":"333"}`, 403)
	tr.expect(admin, "PUT", "/projetos/"+p2+"/integrantes", `{"rgm":" "}`, 400)
	tr.expect(admin, "PUT", "/projetos/inexistente/integrantes", `{"rgm":"333"}`, 404)
	tr.expect(admin, "PUT", "/projetos/"+p2+"/integrantes", `{"rgm":"111"}`, 409)
	tr.expect(admin, "PUT", "/projetos/"+p2+"/integrantes", `{"rgm":"999"}`, 404)
	tr.expect(admin, "PUT", "/projetos/"+p2+"/integrantes", `{"rgm":"666"}`, 200)
	tr.expect(admin, "PUT", "/projetos/"+p2+"/integrantes", `{"rgm":"666"}`, 409)
	tr.expect(aluno("111"), "DELETE", "/projetos/"+p2+"/integrantes", `{"rgm":"666"}`, 403)
	tr.expect(admin, "DELETE", "/projetos/"+p2+"/integrantes", `{"rgm":"333"}`, 404)
	tr.expect(admin, "DELETE", "/projetos/inexistente/integrantes", `{"rgm":"333"}`, 404)
	tr.expect(admin, "DELETE", "/projetos/"+p2+"/integrantes", `{"rgm":"666"}`, 200)

	// convites
	tr.expect(admin, "GET", "/meus-convites", "", 403)
	tr.expect(aluno("222"), "GET", "/meus-convites", "", 200)
	tr.expect(aluno("111"), "GET", "/projetos/"+p1+"/convites", "", 200)
	tr.expect(aluno("333"), "GET", "/projetos/"+p1+"/convites", "", 403)
	tr.expect(prof, "GET", "/projetos/"+p1+"/convites", "", 200)
	tr.expect(aluno("333"), "POST", "/projetos/"+p1+"/convites", `{"rgm":"444"}`, 403)
	tr.expect(aluno("111"), "POST", "/projetos/"+p1+"/convites", `{"rgm":""}`, 400)
	tr.expect(aluno("111"), "POST", "/projetos/"+p1+"/convites", `{"rgm":"111"}`, 409)
	tr.expect(aluno("111"), "POST", "/projetos/"+p1+"/convites", `{"rgm":"888"}`, 400)
	tr.expect(aluno("111"), "POST", "/projetos/"+p1+"/convites", `{"rgm":"222"}`, 409)
	tr.expect(aluno("111"), "POST", "/projetos/"+p1+"/convites", `{"rgm":"333"}`, 201)
	tr.expect(aluno("111"), "POST", "/projetos/inexistente/convites", `{"rgm":"333"}`, 404)
	tr.expect(aluno("333"), "DELETE", "/projetos/"+p1+"/convites/333", "", 403)
	tr.expect(aluno("111"), "DELETE", "/projetos/"+p1+"/convites/444", "", 404)
	tr.expect(aluno("111"), "DELETE", "/projetos/"+p1+"/convites/333", "", 200)
	tr.expect(aluno("333"), "POST", "/meus-convites/"+p1+"/aceitar", "", 404)
	tr.expect(admin, "POST", "/meus-convites/"+p1+"/aceitar", "", 403)
	tr.expect(aluno("222"), "POST", "/meus-convites/"+p1+"/aceitar", "", 200)
	tr.expect(aluno("222"), "POST", "/meus-convites/"+p1+"/recusar", "", 404)
	tr.expect(aluno("111"), "POST", "/projetos/"+p1+"/convites", `{"rgm":"333"}`, 201)
	tr.expect(aluno("333"), "POST", "/meus-convites/"+p1+"/recusar", "", 200)
	tr.expect(aluno("111"), "POST", "/projetos/"+p1+"/convites", `{"rgm":"444"}`, 201)
	tr.expect(aluno("444"), "GET", "/meus-convites", "", 200)

	// atividades
	tr.expect(aluno("111"), "POST", "/atividades", `{"titulo":"Tema","prazo":"2099-11-01T23:59"}`, 403)
	tr.expect(admin, "POST", "/atividades", `{"titulo":"","prazo":"2099-11-01T23:59"}`, 400)
	tr.expect(admin, "POST", "/atividades", `{"titulo":"Tema","prazo":"amanhã"}`, 400)
	tr.expect(admin, "POST", "/atividades", `{"titulo":"Sem campos","prazo":"2099-11-01T23:59","campos":[]}`, 201)
	a1 := id(t, tr.expect(prof, "POST", "/atividades", `{"titulo":"Definição do Tema","descricao":"d","prazo":"2099-11-01T23:59"}`, 201))
	a2body := tr.expect(admin, "POST", "/atividades", `{"titulo":"Revisão","prazo":"2099-12-01T23:59","campos":[{"rotulo":"Repositório","tipo":"LINK","obrigatorio":true},{"rotulo":"Resumo","tipo":"TEXTO_LONGO"}]}`, 201)
	a2 := id(t, a2body)
	linkField, summaryField := jsonField(t, a2body, "campos", 0, "id"), jsonField(t, a2body, "campos", 1, "id")
	tr.expect(aluno("111"), "GET", "/atividades", "", 200)
	tr.expect(aluno("111"), "PUT", "/atividades/"+a1, `{"titulo":"x","prazo":"2099-11-01T23:59"}`, 403)
	tr.expect(admin, "PUT", "/atividades/inexistente", `{"titulo":"x","prazo":"2099-11-01T23:59"}`, 404)
	tr.expect(admin, "PUT", "/atividades/"+a1, `{"titulo":"Tema final","descricao":"","prazo":"2099-11-02T23:59"}`, 200)
	tr.expect(aluno("111"), "POST", "/atividades/"+a1+"/campos", `{"rotulo":"X","tipo":"TEXTO"}`, 403)
	tr.expect(admin, "POST", "/atividades/"+a1+"/campos", `{"rotulo":"X","tipo":"VIDEO"}`, 400)
	tr.expect(admin, "POST", "/atividades/inexistente/campos", `{"rotulo":"X","tipo":"TEXTO"}`, 404)
	extraField := id(t, tr.expect(admin, "POST", "/atividades/"+a1+"/campos", `{"rotulo":"Observações","tipo":"TEXTO"}`, 201))
	tr.expect(aluno("111"), "DELETE", "/atividades/"+a1+"/campos/"+extraField, "", 403)
	tr.expect(admin, "DELETE", "/atividades/"+a1+"/campos/inexistente", "", 404)
	tr.expect(admin, "DELETE", "/atividades/"+a1+"/campos/"+extraField, "", 200)
	tr.expect(admin, "GET", "/atividades", "", 200)

	// entregas
	tr.expect(aluno("333"), "PUT", "/projetos/"+p1+"/entregas/"+a2, `{"respostas":{}}`, 403)
	tr.expect(aluno("111"), "PUT", "/projetos/"+p1+"/entregas/"+a2, `{"respostas":{}}`, 400)
	tr.expect(aluno("111"), "PUT", "/projetos/"+p1+"/entregas/"+a2, `{"respostas":{"`+linkField+`":"não é link"}}`, 400)
	tr.expect(aluno("111"), "PUT", "/projetos/"+p1+"/entregas/inexistente", `{"respostas":{}}`, 404)
	tr.expect(aluno("111"), "PUT", "/projetos/inexistente/entregas/"+a2, `{"respostas":{}}`, 404)
	tr.expect(aluno("222"), "PUT", "/projetos/"+p1+"/entregas/"+a2, `{"respostas":{"`+linkField+`":"https://github.com/x/y","`+summaryField+`":"resumo"}}`, 200)
	tr.expect(aluno("111"), "GET", "/projetos/"+p1+"/entregas", "", 200)
	tr.expect(aluno("333"), "GET", "/projetos/"+p1+"/entregas", "", 403)
	tr.expect(prof, "GET", "/projetos/"+p1+"/entregas", "", 200)
	tr.expect(aluno("111"), "GET", "/atividades/"+a2+"/entregas", "", 403)
	tr.expect(prof, "GET", "/atividades/"+a2+"/entregas", "", 403)
	tr.expect(aluno("111"), "GET", "/programas/"+prog+"/entregas", "", 403)
	tr.expect(admin, "GET", "/programas/"+prog+"/entregas", "", 200)
	tr.expect(prof, "GET", "/programas/"+prog+"/entregas", "", 200)
	tr.expect(prof, "DELETE", "/projetos/"+p2+"/entregas/"+a2, "", 403)
	tr.expect(admin, "GET", "/atividades/"+a2+"/entregas", "", 200)
	tr.expect(aluno("111"), "DELETE", "/projetos/"+p1+"/entregas/"+a2, "", 403)
	tr.expect(prof, "DELETE", "/projetos/"+p1+"/entregas/"+a2, "", 200)
	tr.expect(prof, "DELETE", "/projetos/"+p1+"/entregas/"+a2, "", 404)
	tr.expect(aluno("111"), "PUT", "/projetos/"+p1+"/entregas/"+a2, `{"respostas":{"`+linkField+`":"https://github.com/x/y"}}`, 200)
	tr.expect(aluno("333"), "POST", "/projetos/"+p1+"/entregas/"+a2+"/arquivos", `{"campoId":"`+linkField+`","nome":"a.pdf","tamanho":1}`, 403)
	tr.expect(aluno("111"), "POST", "/projetos/"+p1+"/entregas/"+a2+"/arquivos", `{"campoId":"`+linkField+`","nome":"a.pdf","tamanho":1}`, 400)
	tr.expect(aluno("111"), "POST", "/projetos/"+p1+"/entregas/"+a2+"/arquivos", `{"campoId":"inexistente","nome":"a.pdf","tamanho":1}`, 404)
	tr.expect(aluno("111"), "POST", "/projetos/"+p1+"/entregas/inexistente/arquivos", `{"campoId":"x","nome":"a.pdf","tamanho":1}`, 404)
	tr.expect(aluno("111"), "GET", "/projetos/"+p1+"/entregas/"+a2+"/arquivos/"+linkField, "", 404)
	tr.expect(aluno("333"), "GET", "/projetos/"+p1+"/entregas/"+a2+"/arquivos/"+linkField, "", 403)

	// materiais
	tr.expect(aluno("111"), "POST", "/materiais", `{"titulo":"X","tipo":"LINK","url":"https://x"}`, 403)
	tr.expect(admin, "POST", "/materiais", `{"titulo":"","tipo":"LINK","url":"https://x"}`, 400)
	tr.expect(admin, "POST", "/materiais", `{"titulo":"X","tipo":"LINK"}`, 400)
	tr.expect(admin, "POST", "/materiais", `{"titulo":"X","tipo":"LINK","url":"https://x","cursoId":"inexistente"}`, 404)
	geral := id(t, tr.expect(admin, "POST", "/materiais", `{"titulo":"Manual do PFC","descricao":"Regras","tipo":"DOCUMENTO","url":"https://umc.br/manual.pdf"}`, 201))
	daEng := id(t, tr.expect(prof, "POST", "/materiais", `{"titulo":"Aula gravada","tipo":"VIDEO","url":"https://youtu.be/x","cursoId":"`+eng+`"}`, 201))
	tr.expect(aluno("111"), "GET", "/materiais", "", 200)
	tr.expect(aluno("888"), "GET", "/materiais", "", 200)
	tr.expect(prof, "GET", "/materiais", "", 200)
	tr.expect(aluno("111"), "GET", "/materiais/"+geral+"/arquivo", "", 404)
	tr.expect(aluno("888"), "GET", "/materiais/"+daEng+"/arquivo", "", 404)
	tr.expect(aluno("111"), "POST", "/materiais/arquivos", `{"nome":"a.pdf","tamanho":1}`, 403)
	tr.expect(prof, "POST", "/materiais/arquivos", `{"nome":"a.exe","tamanho":1}`, 400)
	tr.expect(prof, "DELETE", "/materiais/"+geral, "", 403)
	tr.expect(aluno("111"), "DELETE", "/materiais/"+daEng, "", 403)
	tr.expect(prof, "DELETE", "/materiais/"+daEng, "", 200)
	tr.expect(admin, "DELETE", "/materiais/"+daEng, "", 404)

	// referências
	tr.expect(aluno("111"), "GET", "/referencias/busca?q=ab", "", 400)
	tr.expect(aluno("111"), "GET", "/referencias/busca?q=human&pagina=0", "", 400)
	tr.expect(aluno("111"), "GET", "/referencias/doi?doi=abc", "", 400)
	tr.expect(aluno("111"), "GET", "/referencias/doi?doi=10.1000/inexistente", "", 404)
	tr.expect(aluno("111"), "GET", "/referencias/doi?doi=https://doi.org/"+knownDOI, "", 200)
	tr.expect(aluno("333"), "GET", "/projetos/"+p1+"/referencias", "", 403)
	tr.expect(aluno("111"), "POST", "/projetos/"+p1+"/referencias", `{"doi":"abc"}`, 400)
	tr.expect(aluno("333"), "POST", "/projetos/"+p1+"/referencias", `{"doi":"`+knownDOI+`"}`, 403)
	tr.expect(aluno("111"), "POST", "/projetos/"+p1+"/referencias", `{"doi":"10.1000/inexistente"}`, 404)
	refBody := tr.expect(aluno("111"), "POST", "/projetos/"+p1+"/referencias", `{"doi":"`+knownDOI+`"}`, 201)
	tr.expect(aluno("222"), "POST", "/projetos/"+p1+"/referencias", `{"doi":"doi:`+knownDOI+`"}`, 409)
	tr.expect(prof, "GET", "/projetos/"+p1+"/referencias", "", 200)
	tr.expect(aluno("111"), "DELETE", "/projetos/"+p1+"/referencias/inexistente", "", 404)
	tr.expect(aluno("111"), "DELETE", "/projetos/"+p1+"/referencias/"+jsonField(t, refBody, "id"), "", 200)
	tr.expect(aluno("111"), "POST", "/projetos/"+p1+"/referencias", `{"doi":"`+knownDOI+`"}`, 201)

	// exclusões em cascata
	tr.expect(admin, "DELETE", "/programas/"+prog, "", 409)
	tr.expect(aluno("111"), "DELETE", "/projetos/"+p1, "", 403)
	tr.expect(admin, "DELETE", "/projetos/"+p1, "", 200)
	tr.expect(admin, "DELETE", "/projetos/"+p1, "", 404)
	tr.expect(aluno("444"), "GET", "/meus-convites", "", 200)
	tr.expect(admin, "GET", "/projetos/"+p1+"/entregas", "", 404)
	tr.expect(admin, "GET", "/atividades/"+a2+"/entregas", "", 200)
	tr.expect(aluno("111"), "POST", "/meu-pfc", `{"nome":"Recomeço"}`, 201)
	tr.expect(aluno("111"), "DELETE", "/atividades/"+a2, "", 403)
	tr.expect(admin, "DELETE", "/atividades/"+a2, "", 200)
	tr.expect(admin, "DELETE", "/atividades/"+a2, "", 404)
	tr.expect(admin, "DELETE", "/programas/inexistente", "", 404)

	// roteador
	tr.expect(admin, "GET", "/nao-existe", "", 404)
	tr.expect(admin, "PATCH", "/turmas", "", 404)
	tr.expect(pub, "OPTIONS", "/turmas", "", 204)

	got := tr.out.Bytes()
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("sem %s — rode com -update: %v", goldenPath, err)
	}
	if !bytes.Equal(got, want) {
		os.WriteFile(goldenPath+".got", got, 0o644)
		t.Fatalf("a API mudou: compare %s com %s.got", goldenPath, goldenPath)
	}
}
