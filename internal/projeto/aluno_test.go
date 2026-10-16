package projeto

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func reqAluno(perfil, email, corpo string) events.APIGatewayProxyRequest {
	return events.APIGatewayProxyRequest{
		Body: corpo,
		RequestContext: events.APIGatewayProxyRequestContext{
			Authorizer: map[string]interface{}{
				"claims": map[string]interface{}{"custom:perfil": perfil, "email": email},
			},
		},
	}
}

// falsos monta os colaboradores do handler. turmas: RGM -> turma;
// ocupados: RGMs que já estão em algum projeto. Devolve os projetos salvos
// e os RGMs que receberam convite.
func falsos(t *testing.T, turmas map[string]string, programas map[string]string, ocupados map[string]bool) (*[]NovoProjeto, *[]string) {
	t.Helper()
	salvos := &[]NovoProjeto{}
	convidados := &[]string{}

	origTurmas, origProg, origOcupados, origSalvar := turmasDosRGMs, programaDaTurma, ocupadosDoGrupo, salvarProjeto
	origConvite, origNotificar := criarConvite, notificarConvite
	t.Cleanup(func() {
		turmasDosRGMs, programaDaTurma, ocupadosDoGrupo, salvarProjeto = origTurmas, origProg, origOcupados, origSalvar
		criarConvite, notificarConvite = origConvite, origNotificar
	})

	criarConvite = func(_ context.Context, projetoID, projetoNome, rgm, por string) (Convite, error) {
		*convidados = append(*convidados, rgm)
		return Convite{ProjetoID: projetoID, ProjetoNome: projetoNome, RGM: rgm, ConvidadoPor: por}, nil
	}
	notificarConvite = func(Convite) {}

	// Os falsos em lote devolvem só os RGMs pedidos, como o BatchGetItem.
	turmasDosRGMs = func(_ context.Context, rgms []string) (map[string]string, error) {
		achadas := map[string]string{}
		for _, r := range rgms {
			if t, ok := turmas[r]; ok {
				achadas[r] = t
			}
		}
		return achadas, nil
	}
	programaDaTurma = func(_ context.Context, turma string) (string, bool, error) {
		id, ok := programas[turma]
		return id, ok, nil
	}
	ocupadosDoGrupo = func(_ context.Context, rgms []string) (map[string]string, error) {
		achados := map[string]string{}
		for _, r := range rgms {
			if ocupados[r] {
				achados[r] = "outro-projeto"
			}
		}
		return achados, nil
	}
	salvarProjeto = func(_ context.Context, programaID string, n NovoProjeto) (Projeto, error) {
		*salvos = append(*salvos, n)
		return Projeto{ID: "p1", Nome: n.Nome, ProgramaID: programaID, Integrantes: n.Integrantes}, nil
	}
	return salvos, convidados
}

func TestCriarDoAlunoSucesso(t *testing.T) {
	salvos, convidados := falsos(t,
		map[string]string{"111": "turma-a", "222": "turma-a"},
		map[string]string{"turma-a": "prog-a"},
		nil)

	resp, _ := HandleCriarDoAluno(context.Background(),
		reqAluno("ALUNO", "111@alunos.umc.br", `{"nome":" Athena ","descricao":"d","integrantes":["222","111"," 222 ",""]}`))
	if resp.StatusCode != 201 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, resp.Body)
	}
	var p Projeto
	if err := json.Unmarshal([]byte(resp.Body), &p); err != nil {
		t.Fatal(err)
	}
	if p.ProgramaID != "prog-a" || p.Nome != "Athena" {
		t.Errorf("projeto inesperado: %+v", p)
	}
	if len(p.Integrantes) != 1 || p.Integrantes[0] != "111" {
		t.Errorf("integrantes = %v, esperado [111] (só o criador entra na hora)", p.Integrantes)
	}
	if len(*convidados) != 1 || (*convidados)[0] != "222" {
		t.Errorf("convidados = %v, esperado [222] (colega vira convite, sem repetição)", *convidados)
	}
	if len(*salvos) != 1 {
		t.Errorf("salvou %d projetos", len(*salvos))
	}
}

func TestCriarDoAlunoRecusas(t *testing.T) {
	turmas := map[string]string{"111": "turma-a", "222": "turma-a", "333": "turma-b", "444": "turma-a"}
	programas := map[string]string{"turma-a": "prog-a"}

	casos := []struct {
		nome     string
		perfil   string
		email    string
		corpo    string
		ocupados map[string]bool
		esperado int
	}{
		{"admin não usa a rota", "ADMIN", "x@umc.br", `{"nome":"A"}`, nil, 403},
		{"orientador não usa a rota", "ORIENTADOR", "x@umc.br", `{"nome":"A"}`, nil, 403},
		{"sem nome", "ALUNO", "111@alunos.umc.br", `{"nome":"  "}`, nil, 400},
		{"corpo inválido", "ALUNO", "111@alunos.umc.br", `{`, nil, 400},
		{"RGM sem turma", "ALUNO", "999@alunos.umc.br", `{"nome":"A"}`, nil, 403},
		{"turma sem programa", "ALUNO", "333@alunos.umc.br", `{"nome":"A"}`, nil, 409},
		{"criador já em projeto", "ALUNO", "111@alunos.umc.br", `{"nome":"A"}`, map[string]bool{"111": true}, 409},
		{"colega de outra turma", "ALUNO", "111@alunos.umc.br", `{"nome":"A","integrantes":["333"]}`, nil, 400},
		{"colega sem pré-autorização", "ALUNO", "111@alunos.umc.br", `{"nome":"A","integrantes":["888"]}`, nil, 400},
		{"colega já em projeto", "ALUNO", "111@alunos.umc.br", `{"nome":"A","integrantes":["222"]}`, map[string]bool{"222": true}, 409},
	}
	for _, c := range casos {
		salvos, convidados := falsos(t, turmas, programas, c.ocupados)
		resp, _ := HandleCriarDoAluno(context.Background(), reqAluno(c.perfil, c.email, c.corpo))
		if resp.StatusCode != c.esperado {
			t.Errorf("%s: status = %d (%s), esperado %d", c.nome, resp.StatusCode, resp.Body, c.esperado)
		}
		if len(*salvos) != 0 || len(*convidados) != 0 {
			t.Errorf("%s: não deveria ter salvado projeto nem convite", c.nome)
		}
	}
}

func TestIntegrantesDoGrupo(t *testing.T) {
	got := integrantesDoGrupo("1", []string{"2", " 2 ", "", "1", "3"})
	if len(got) != 3 || got[0] != "1" || got[1] != "2" || got[2] != "3" {
		t.Errorf("grupo = %v", got)
	}
}
