package curso

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/google/uuid"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// Modelagem (mesma tabela single-table dos demais domínios):
//
//	turma            PK=CURSO#<id>                       SK=PROFILE      GSI1PK=TURMAS  GSI1SK=CURSO#<id>
//	trava de turma   PK=TURMA_UNICA#<nome>#<turno>#<per>  SK=TURMA_UNICA  cursoId
//	programa da turma PK=CURSO#<id>                      SK=PROGRAMA     programaId (gravado pelo pacote programa)
//
// O GSI1 lista as turmas sem Scan. A trava existe só pra o DynamoDB recusar,
// numa transação, uma segunda turma com o mesmo curso+turno+período — a
// checagem antiga (Scan e depois Put) deixava duas requisições simultâneas
// criarem a mesma turma.

var (
	errCursoComPrograma = errors.New("curso possui programa vinculado")
	errTurmaDuplicada   = errors.New("já existe turma com esse curso, turno e período")
	errNaoEncontrada    = errors.New("turma não encontrada")
	errTurmaMudou       = errors.New("a turma mudou durante a operação")
)

type item struct {
	PK      string `dynamodbav:"PK"`
	SK      string `dynamodbav:"SK"`
	GSI1PK  string `dynamodbav:"GSI1PK"`
	GSI1SK  string `dynamodbav:"GSI1SK"`
	Nome    string `dynamodbav:"nome"`
	Turno   string `dynamodbav:"turno"`
	Periodo string `dynamodbav:"periodo"`
}

func chaveTurma(id string) common.Item { return common.Chave("CURSO#"+id, "PROFILE") }

// chaveTrava identifica a combinação curso+turno+período.
func chaveTrava(nome, turno, periodo string) common.Item {
	return common.Chave(fmt.Sprintf("TURMA_UNICA#%s#%s#%s", nome, turno, periodo), "TURMA_UNICA")
}

// ChavePrograma é o item que liga a turma ao programa dela — gravado pelo
// pacote programa, conferido aqui antes de apagar a turma.
func ChavePrograma(cursoID string) common.Item { return common.Chave("CURSO#"+cursoID, "PROGRAMA") }

func toCurso(it item) Curso {
	return Curso{
		ID:      it.PK[len("CURSO#"):],
		Nome:    it.Nome,
		Turno:   it.Turno,
		Periodo: it.Periodo,
	}
}

func buscar(ctx context.Context, id string) (item, bool, error) {
	return common.Buscar[item](ctx, chaveTurma(id))
}

// salvar cria a turma e a trava dela na mesma transação.
func salvar(ctx context.Context, novo NovoCurso) (Curso, error) {
	id := uuid.NewString()
	it := item{
		PK:      "CURSO#" + id,
		SK:      "PROFILE",
		GSI1PK:  "TURMAS",
		GSI1SK:  "CURSO#" + id,
		Nome:    novo.Nome,
		Turno:   novo.Turno,
		Periodo: novo.Periodo,
	}
	av, err := attributevalue.MarshalMap(it)
	if err != nil {
		return Curso{}, fmt.Errorf("falha ao serializar turma: %w", err)
	}
	trava := chaveTrava(novo.Nome, novo.Turno, novo.Periodo)
	trava["cursoId"] = common.S(id)

	err = common.Transacao(ctx,
		common.Put(av, "attribute_not_exists(PK)", nil),
		common.Put(trava, "attribute_not_exists(PK)", nil),
	)
	if common.FalhouItem(err, 1) {
		return Curso{}, errTurmaDuplicada
	}
	if err != nil {
		return Curso{}, err
	}
	return toCurso(it), nil
}

// Existe diz se há uma turma com este id.
func Existe(ctx context.Context, id string) (bool, error) {
	return common.Existe(ctx, chaveTurma(id))
}

func listar(ctx context.Context) ([]Curso, error) {
	itens, err := common.QueryNoIndice[item](ctx, "TURMAS")
	if err != nil {
		return nil, err
	}
	cursos := make([]Curso, 0, len(itens))
	for _, it := range itens {
		cursos = append(cursos, toCurso(it))
	}
	return cursos, nil
}

// atualizar troca curso/turno/período. Se a combinação mudou, a trava velha
// sai e a nova entra na mesma transação; a condição na turma garante que
// ninguém a alterou entre a leitura e a escrita.
func atualizar(ctx context.Context, id string, dados AtualizarCurso) error {
	atual, ok, err := buscar(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return errNaoEncontrada
	}
	if atual.Nome == dados.Nome && atual.Turno == dados.Turno && atual.Periodo == dados.Periodo {
		return nil
	}

	nova := chaveTrava(dados.Nome, dados.Turno, dados.Periodo)
	nova["cursoId"] = common.S(id)
	err = common.Transacao(ctx,
		common.Update(chaveTurma(id),
			"SET nome = :n, turno = :t, periodo = :p",
			"nome = :vn AND turno = :vt AND periodo = :vp",
			common.Item{
				":n": common.S(dados.Nome), ":t": common.S(dados.Turno), ":p": common.S(dados.Periodo),
				":vn": common.S(atual.Nome), ":vt": common.S(atual.Turno), ":vp": common.S(atual.Periodo),
			}),
		common.Delete(chaveTrava(atual.Nome, atual.Turno, atual.Periodo),
			"attribute_not_exists(PK) OR cursoId = :id", common.Item{":id": common.S(id)}),
		common.Put(nova, "attribute_not_exists(PK)", nil),
	)
	switch {
	case common.FalhouItem(err, 2):
		return errTurmaDuplicada
	case common.FalhouItem(err, 0), common.FalhouItem(err, 1):
		return errTurmaMudou
	}
	return err
}

// deletar apaga a turma e a trava, recusando se ela tiver programa — a
// conferência é feita na própria transação, sem Scan.
func deletar(ctx context.Context, id string) error {
	atual, ok, err := buscar(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return errNaoEncontrada
	}
	err = common.Transacao(ctx,
		common.Delete(chaveTurma(id), "attribute_exists(PK)", nil),
		common.Delete(chaveTrava(atual.Nome, atual.Turno, atual.Periodo),
			"attribute_not_exists(PK) OR cursoId = :id", common.Item{":id": common.S(id)}),
		common.Conferir(ChavePrograma(id), "attribute_not_exists(PK)", nil),
	)
	switch {
	case common.FalhouItem(err, 2):
		return errCursoComPrograma
	case common.FalhouItem(err, 0):
		return errNaoEncontrada
	case common.FalhouItem(err, 1):
		return errTurmaMudou
	}
	return err
}
