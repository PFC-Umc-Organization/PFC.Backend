package projeto

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

// cenario é um "banco de mentira" em memória pros testes de convite.
// convites usa a chave "<projetoId>|<rgm>".
type cenario struct {
	convites  map[string]bool
	projetos  map[string]Projeto
	ocupados  map[string]bool
	turmas    map[string]string // RGM -> turma
	programas map[string]string // turma -> programa
	cheio     bool              // entrarComConvite recusa (grupo cheio)
	entrou    []string          // "<projetoId>|<rgm>" de quem entrou
}

// novoCenario: o aluno 222 (turma-a) tem convites de p1 e p2; p1 é do
// programa da turma-a e tem só o 111.
func novoCenario() *cenario {
	return &cenario{
		convites:  map[string]bool{"p1|222": true, "p2|222": true},
		projetos:  map[string]Projeto{"p1": {ID: "p1", Nome: "Athena", ProgramaID: "prog-a", Integrantes: []string{"111"}}},
		ocupados:  map[string]bool{},
		turmas:    map[string]string{"111": "turma-a", "222": "turma-a", "333": "turma-b", "444": "turma-a", "555": "turma-a"},
		programas: map[string]string{"turma-a": "prog-a", "turma-b": "prog-b"},
	}
}

// instalar troca as funções do banco pelas falsas deste cenário e devolve
// as originais no fim do teste.
func (c *cenario) instalar(t *testing.T) {
	t.Helper()
	origConvite, origRecebidos, origRemover := conviteDoAluno, convitesRecebidos, removerConvite
	origProjeto, origEntrar := projetoPorID, entrarComConvite
	origOcupado, origTurma, origPrograma := projetoDoRGM, turmaDoRGM, programaDaTurma
	origDoGrupo, origCriar, origNotificar := convitesDoGrupo, criarConvite, notificarConvite
	t.Cleanup(func() {
		conviteDoAluno, convitesRecebidos, removerConvite = origConvite, origRecebidos, origRemover
		projetoPorID, entrarComConvite = origProjeto, origEntrar
		projetoDoRGM, turmaDoRGM, programaDaTurma = origOcupado, origTurma, origPrograma
		convitesDoGrupo, criarConvite, notificarConvite = origDoGrupo, origCriar, origNotificar
	})

	convitesDoGrupo = func(_ context.Context, projetoID string) ([]Convite, error) {
		lista := []Convite{}
		for chave := range c.convites {
			doProjeto, rgm, _ := strings.Cut(chave, "|")
			if doProjeto == projetoID {
				lista = append(lista, Convite{ProjetoID: projetoID, RGM: rgm})
			}
		}
		return lista, nil
	}
	criarConvite = func(_ context.Context, projetoID, projetoNome, rgm, por string) (Convite, error) {
		chave := projetoID + "|" + rgm
		if c.convites[chave] {
			return Convite{}, errConviteJaExiste
		}
		c.convites[chave] = true
		return Convite{ProjetoID: projetoID, ProjetoNome: projetoNome, RGM: rgm, ConvidadoPor: por}, nil
	}
	notificarConvite = func(Convite) {}

	conviteDoAluno = func(_ context.Context, projetoID, rgm string) (Convite, bool, error) {
		return Convite{ProjetoID: projetoID, RGM: rgm}, c.convites[projetoID+"|"+rgm], nil
	}
	convitesRecebidos = func(_ context.Context, rgm string) ([]Convite, error) {
		lista := []Convite{}
		for chave := range c.convites {
			projetoID, dono, _ := strings.Cut(chave, "|")
			if dono == rgm {
				lista = append(lista, Convite{ProjetoID: projetoID, RGM: rgm})
			}
		}
		return lista, nil
	}
	removerConvite = func(_ context.Context, projetoID, rgm string) error {
		chave := projetoID + "|" + rgm
		if !c.convites[chave] {
			return errConviteNaoExiste
		}
		delete(c.convites, chave)
		return nil
	}
	projetoPorID = func(_ context.Context, projetoID string) (Projeto, bool, error) {
		p, ok := c.projetos[projetoID]
		return p, ok, nil
	}
	entrarComConvite = func(_ context.Context, projetoID, rgm string, _ []string) error {
		if c.cheio {
			return errNaoPodeEntrar
		}
		c.entrou = append(c.entrou, projetoID+"|"+rgm)
		return nil
	}
	projetoDoRGM = func(_ context.Context, rgm string) (Projeto, bool, error) {
		return Projeto{}, c.ocupados[rgm], nil
	}
	turmaDoRGM = func(_ context.Context, rgm string) (string, error) { return c.turmas[rgm], nil }
	programaDaTurma = func(_ context.Context, turma string) (string, bool, error) {
		id, ok := c.programas[turma]
		return id, ok, nil
	}
}

func reqConvite(perfil, email, projetoID string) events.APIGatewayProxyRequest {
	r := reqAluno(perfil, email, "")
	r.PathParameters = map[string]string{"projetoId": projetoID}
	return r
}

func TestAceitarConviteSucesso(t *testing.T) {
	c := novoCenario()
	c.instalar(t)

	resp, _ := HandleAceitarConvite(context.Background(), reqConvite("ALUNO", "222@alunos.umc.br", "p1"))
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, resp.Body)
	}
	var p Projeto
	if err := json.Unmarshal([]byte(resp.Body), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Integrantes) != 2 || p.Integrantes[1] != "222" {
		t.Errorf("integrantes = %v, esperado [111 222]", p.Integrantes)
	}
	if len(c.entrou) != 1 || c.entrou[0] != "p1|222" {
		t.Errorf("entrou = %v, esperado [p1|222]", c.entrou)
	}
	if len(c.convites) != 0 {
		t.Errorf("sobraram convites %v — o aceite deveria apagar todos os do aluno", c.convites)
	}
}

func TestAceitarConviteRecusas(t *testing.T) {
	casos := []struct {
		nome     string
		perfil   string
		email    string
		projeto  string
		ajuste   func(c *cenario)
		esperado int
	}{
		{"orientador não usa a rota", "ORIENTADOR", "x@umc.br", "p1", nil, 403},
		{"sem convite", "ALUNO", "333@alunos.umc.br", "p1", nil, 404},
		{"grupo apagado", "ALUNO", "222@alunos.umc.br", "p2", nil, 404},
		{"já está em um projeto", "ALUNO", "222@alunos.umc.br", "p1", func(c *cenario) { c.ocupados["222"] = true }, 409},
		{"mudou de turma", "ALUNO", "222@alunos.umc.br", "p1", func(c *cenario) { c.turmas["222"] = "turma-b" }, 409},
		{"grupo cheio", "ALUNO", "222@alunos.umc.br", "p1", func(c *cenario) { c.cheio = true }, 409},
	}
	for _, caso := range casos {
		c := novoCenario()
		if caso.ajuste != nil {
			caso.ajuste(c)
		}
		c.instalar(t)

		resp, _ := HandleAceitarConvite(context.Background(), reqConvite(caso.perfil, caso.email, caso.projeto))
		if resp.StatusCode != caso.esperado {
			t.Errorf("%s: status = %d (%s), esperado %d", caso.nome, resp.StatusCode, resp.Body, caso.esperado)
		}
		if len(c.entrou) != 0 {
			t.Errorf("%s: não deveria ter entrado em grupo nenhum", caso.nome)
		}
	}
}

func TestAceitarConviteDeGrupoApagadoLimpaOConvite(t *testing.T) {
	c := novoCenario() // p2 não existe em c.projetos
	c.instalar(t)

	HandleAceitarConvite(context.Background(), reqConvite("ALUNO", "222@alunos.umc.br", "p2"))
	if c.convites["p2|222"] {
		t.Error("o convite órfão do grupo apagado deveria ter sido removido")
	}
	if !c.convites["p1|222"] {
		t.Error("o convite de p1 não deveria ter sido mexido")
	}
}

func TestRecusarConvite(t *testing.T) {
	c := novoCenario()
	c.instalar(t)

	resp, _ := HandleRecusarConvite(context.Background(), reqConvite("ALUNO", "222@alunos.umc.br", "p1"))
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, resp.Body)
	}
	if c.convites["p1|222"] {
		t.Error("o convite recusado deveria ter sido apagado")
	}
	if len(c.entrou) != 0 {
		t.Error("recusar não pode pôr ninguém no grupo")
	}

	resp, _ = HandleRecusarConvite(context.Background(), reqConvite("ALUNO", "222@alunos.umc.br", "p1"))
	if resp.StatusCode != 404 {
		t.Errorf("recusar de novo: status = %d, esperado 404", resp.StatusCode)
	}
}

func TestListarMeusConvites(t *testing.T) {
	c := novoCenario()
	c.convites["p1|333"] = true
	c.instalar(t)

	resp, _ := HandleListarMeusConvites(context.Background(), reqConvite("ALUNO", "333@alunos.umc.br", ""))
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, resp.Body)
	}
	var convites []Convite
	if err := json.Unmarshal([]byte(resp.Body), &convites); err != nil {
		t.Fatal(err)
	}
	if len(convites) != 1 || convites[0].ProjetoID != "p1" || convites[0].RGM != "333" {
		t.Errorf("convites = %+v, esperado só o de p1 pro 333", convites)
	}
}

func reqGrupo(perfil, email, projetoID, rgm, corpo string) events.APIGatewayProxyRequest {
	r := reqAluno(perfil, email, corpo)
	r.PathParameters = map[string]string{"projetoId": projetoID, "rgm": rgm}
	return r
}

func TestConvidar(t *testing.T) {
	casos := []struct {
		nome     string
		perfil   string
		email    string
		projeto  string
		corpo    string
		ajuste   func(c *cenario)
		esperado int
	}{
		{"integrante convida colega livre", "ALUNO", "111@alunos.umc.br", "p1", `{"rgm":" 444 "}`, nil, 201},
		{"projeto inexistente", "ALUNO", "111@alunos.umc.br", "p9", `{"rgm":"444"}`, nil, 404},
		{"aluno de fora do grupo", "ALUNO", "222@alunos.umc.br", "p1", `{"rgm":"444"}`, nil, 403},
		{"orientador não convida", "ORIENTADOR", "x@umc.br", "p1", `{"rgm":"444"}`, nil, 403},
		{"rgm vazio", "ALUNO", "111@alunos.umc.br", "p1", `{"rgm":"  "}`, nil, 400},
		{"já é integrante", "ALUNO", "111@alunos.umc.br", "p1", `{"rgm":"111"}`, nil, 409},
		{"convidado de outra turma", "ALUNO", "111@alunos.umc.br", "p1", `{"rgm":"333"}`, nil, 400},
		{"convidado já em projeto", "ALUNO", "111@alunos.umc.br", "p1", `{"rgm":"555"}`, func(c *cenario) { c.ocupados["555"] = true }, 409},
		{"convite repetido", "ALUNO", "111@alunos.umc.br", "p1", `{"rgm":"222"}`, nil, 409},
		{"grupo cheio contando pendentes", "ALUNO", "111@alunos.umc.br", "p1", `{"rgm":"444"}`, func(c *cenario) {
			p := c.projetos["p1"]
			p.Integrantes = []string{"111", "a", "b", "c", "d", "e", "f", "g", "h"} // 9 + o convite do 222 = 10
			c.projetos["p1"] = p
		}, 409},
	}
	for _, caso := range casos {
		c := novoCenario()
		if caso.ajuste != nil {
			caso.ajuste(c)
		}
		c.instalar(t)
		antes := len(c.convites)

		resp, _ := HandleConvidar(context.Background(), reqGrupo(caso.perfil, caso.email, caso.projeto, "", caso.corpo))
		if resp.StatusCode != caso.esperado {
			t.Errorf("%s: status = %d (%s), esperado %d", caso.nome, resp.StatusCode, resp.Body, caso.esperado)
		}

		criou := len(c.convites) - antes
		if caso.esperado == 201 {
			if !c.convites["p1|444"] {
				t.Errorf("%s: deveria ter criado o convite p1|444 (com o RGM aparado)", caso.nome)
			}
		} else if criou != 0 {
			t.Errorf("%s: não deveria ter criado convite", caso.nome)
		}
	}
}

func TestListarConvitesDoGrupo(t *testing.T) {
	casos := []struct {
		nome     string
		perfil   string
		email    string
		esperado int
	}{
		{"integrante vê", "ALUNO", "111@alunos.umc.br", 200},
		{"orientador vê", "ORIENTADOR", "x@umc.br", 200},
		{"aluno de fora não vê", "ALUNO", "333@alunos.umc.br", 403},
	}
	for _, caso := range casos {
		c := novoCenario()
		c.instalar(t)

		resp, _ := HandleListarConvitesDoGrupo(context.Background(), reqGrupo(caso.perfil, caso.email, "p1", "", ""))
		if resp.StatusCode != caso.esperado {
			t.Errorf("%s: status = %d (%s), esperado %d", caso.nome, resp.StatusCode, resp.Body, caso.esperado)
			continue
		}
		if caso.esperado == 200 {
			var convites []Convite
			if err := json.Unmarshal([]byte(resp.Body), &convites); err != nil {
				t.Fatal(err)
			}
			if len(convites) != 1 || convites[0].RGM != "222" {
				t.Errorf("%s: convites = %+v, esperado só o do 222 (o de p2 é de outro grupo)", caso.nome, convites)
			}
		}
	}
}

func TestCancelarConvite(t *testing.T) {
	c := novoCenario()
	c.instalar(t)

	resp, _ := HandleCancelarConvite(context.Background(), reqGrupo("ALUNO", "222@alunos.umc.br", "p1", "222", ""))
	if resp.StatusCode != 403 {
		t.Errorf("quem não é integrante cancelando: status = %d, esperado 403", resp.StatusCode)
	}
	if !c.convites["p1|222"] {
		t.Fatal("o 403 não pode ter apagado o convite")
	}

	resp, _ = HandleCancelarConvite(context.Background(), reqGrupo("ALUNO", "111@alunos.umc.br", "p1", "222", ""))
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, resp.Body)
	}
	if c.convites["p1|222"] {
		t.Error("o convite cancelado deveria ter sido apagado")
	}

	resp, _ = HandleCancelarConvite(context.Background(), reqGrupo("ALUNO", "111@alunos.umc.br", "p1", "222", ""))
	if resp.StatusCode != 404 {
		t.Errorf("cancelar de novo: status = %d, esperado 404", resp.StatusCode)
	}
}
