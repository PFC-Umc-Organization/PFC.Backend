package programa

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/curso"
)

// Modelagem (mesma tabela single-table dos demais domínios):
//
//	programa           PK=PROGRAM#<id>  SK=PROFILE   GSI1PK=PROGRAMAS  GSI1SK=PROGRAM#<id>  cursoId
//	programa da turma  PK=CURSO#<cid>   SK=PROGRAMA  programaId
//
// O segundo item é o vínculo turma → programa: responde "qual o programa
// desta turma?" com um GetItem (era um Scan da tabela inteira, feito em todo
// /meu-pfc, convite e aceite) e, criado com attribute_not_exists na mesma
// transação do programa, garante um programa por turma.

var (
	errProgramaComProjetos = errors.New("programa possui projetos vinculados")
	errJaExisteParaCurso   = errors.New("este curso já possui um programa de PFC")
	errCursoNaoExiste      = errors.New("curso não encontrado")
	errNaoEncontrado       = errors.New("programa não encontrado")
	errProgramaMudou       = errors.New("o programa mudou durante a operação")
)

type item struct {
	PK      string `dynamodbav:"PK"`
	SK      string `dynamodbav:"SK"`
	GSI1PK  string `dynamodbav:"GSI1PK"`
	GSI1SK  string `dynamodbav:"GSI1SK"`
	CursoID string `dynamodbav:"cursoId"`
}

type itemVinculo struct {
	ProgramaID string `dynamodbav:"programaId"`
}

func chavePrograma(id string) common.Item { return common.Chave("PROGRAM#"+id, "PROFILE") }

func vinculo(cursoID, programaID string) common.Item {
	v := curso.ChavePrograma(cursoID)
	v["programaId"] = common.S(programaID)
	return v
}

func toPrograma(it item) Programa {
	return Programa{ID: it.PK[len("PROGRAM#"):], CursoID: it.CursoID}
}

// IDDoCurso devolve o id do Programa de PFC da turma (curso). ok=false
// quando a turma ainda não tem programa.
func IDDoCurso(ctx context.Context, cursoID string) (id string, ok bool, err error) {
	if cursoID == "" {
		return "", false, nil
	}
	v, ok, err := common.Buscar[itemVinculo](ctx, curso.ChavePrograma(cursoID), "programaId")
	return v.ProgramaID, ok, err
}

func buscar(ctx context.Context, id string) (item, bool, error) {
	return common.Buscar[item](ctx, chavePrograma(id))
}

// salvar cria o programa. A transação confere que a turma existe e que ela
// ainda não tem programa.
func salvar(ctx context.Context, novo NovoPrograma) (Programa, error) {
	id := uuid.NewString()
	it := item{
		PK:      "PROGRAM#" + id,
		SK:      "PROFILE",
		GSI1PK:  "PROGRAMAS",
		GSI1SK:  "PROGRAM#" + id,
		CursoID: novo.CursoID,
	}
	av, err := attributevalue.MarshalMap(it)
	if err != nil {
		return Programa{}, fmt.Errorf("falha ao serializar programa: %w", err)
	}

	err = common.Transacao(ctx,
		common.Conferir(common.Chave("CURSO#"+novo.CursoID, "PROFILE"), "attribute_exists(PK)", nil),
		common.Put(vinculo(novo.CursoID, id), "attribute_not_exists(PK)", nil),
		common.Put(av, "attribute_not_exists(PK)", nil),
	)
	switch {
	case common.FalhouItem(err, 0):
		return Programa{}, errCursoNaoExiste
	case common.FalhouItem(err, 1):
		return Programa{}, errJaExisteParaCurso
	case err != nil:
		return Programa{}, err
	}
	return toPrograma(it), nil
}

func listar(ctx context.Context) ([]Programa, error) {
	itens, err := common.QueryNoIndice[item](ctx, "PROGRAMAS")
	if err != nil {
		return nil, err
	}
	programas := make([]Programa, 0, len(itens))
	for _, it := range itens {
		programas = append(programas, toPrograma(it))
	}
	return programas, nil
}

// Existe diz se há um programa com este id.
func Existe(ctx context.Context, id string) (bool, error) {
	return common.Existe(ctx, chavePrograma(id))
}

// atualizar move o programa pra outra turma: o vínculo da turma antiga sai
// e o da nova entra, tudo na mesma transação.
func atualizar(ctx context.Context, id string, dados AtualizarPrograma) error {
	atual, ok, err := buscar(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return errNaoEncontrado
	}
	if atual.CursoID == dados.CursoID {
		return nil
	}

	err = common.Transacao(ctx,
		common.Update(chavePrograma(id), "SET cursoId = :novo", "cursoId = :velho",
			common.Item{":novo": common.S(dados.CursoID), ":velho": common.S(atual.CursoID)}),
		common.Delete(curso.ChavePrograma(atual.CursoID),
			"attribute_not_exists(PK) OR programaId = :id", common.Item{":id": common.S(id)}),
		common.Conferir(common.Chave("CURSO#"+dados.CursoID, "PROFILE"), "attribute_exists(PK)", nil),
		common.Put(vinculo(dados.CursoID, id), "attribute_not_exists(PK)", nil),
	)
	switch {
	case common.FalhouItem(err, 2):
		return errCursoNaoExiste
	case common.FalhouItem(err, 3):
		return errJaExisteParaCurso
	case common.FalhouItem(err, 0), common.FalhouItem(err, 1):
		return errProgramaMudou
	}
	return err
}

func possuiProjetos(ctx context.Context, id string) (bool, error) {
	out, err := common.DynamoDB().Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(common.Tabela),
		IndexName:                 aws.String(common.Indice),
		KeyConditionExpression:    aws.String("GSI1PK = :programa"),
		ExpressionAttributeValues: common.Item{":programa": common.S("PROGRAM#" + id)},
		ProjectionExpression:      aws.String("PK"),
		Limit:                     aws.Int32(1),
	})
	if err != nil {
		return false, err
	}
	return len(out.Items) > 0, nil
}

// deletar apaga o programa e o vínculo com a turma. A checagem de projetos
// ainda é ler-depois-escrever (não dá pra conferir um GSI numa transação):
// um projeto criado no mesmo instante escapa — risco aceito, a operação é
// rara e feita só pelo admin.
func deletar(ctx context.Context, id string) error {
	atual, ok, err := buscar(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return errNaoEncontrado
	}
	temProjetos, err := possuiProjetos(ctx, id)
	if err != nil {
		return err
	}
	if temProjetos {
		return errProgramaComProjetos
	}

	err = common.Transacao(ctx,
		common.Delete(chavePrograma(id), "attribute_exists(PK)", nil),
		common.Delete(curso.ChavePrograma(atual.CursoID),
			"attribute_not_exists(PK) OR programaId = :id", common.Item{":id": common.S(id)}),
	)
	if common.FalhouItem(err, 0) {
		return errNaoEncontrado
	}
	if common.FalhouItem(err, 1) {
		return errProgramaMudou
	}
	return err
}
