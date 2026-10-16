package projeto

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// Modelagem (mesma tabela single-table dos demais domínios):
//
//	projeto  PK=PROJECT#<id>  SK=PROFILE  GSI1PK=PROGRAM#<pid>  GSI1SK=PROJECT#<id>  integrantes, ...
//	vínculo  PK=MEMBRO#<rgm>  SK=PROJETO  projetoId
//
// O vínculo diz em que projeto o aluno está: "projeto do RGM" vira um
// GetItem (era um Scan da tabela inteira) e, como ele é gravado com
// attribute_not_exists na mesma transação que põe o RGM em `integrantes`,
// o DynamoDB recusa o aluno num segundo projeto mesmo com duas requisições
// simultâneas. Toda escrita em `integrantes` mexe no vínculo junto.

var (
	errProjetoNaoExiste  = errors.New("projeto não encontrado")
	errNaoIntegrante     = errors.New("RGM não é integrante do projeto")
	errGrupoMudou        = errors.New("o grupo mudou durante a operação")
	errAlunoJaTemProjeto = errors.New("o aluno já faz parte de um projeto")
)

type item struct {
	PK           string   `dynamodbav:"PK"`
	SK           string   `dynamodbav:"SK"`
	GSI1PK       string   `dynamodbav:"GSI1PK"`
	GSI1SK       string   `dynamodbav:"GSI1SK"`
	Nome         string   `dynamodbav:"nome"`
	Descricao    string   `dynamodbav:"descricao"`
	ProgramaID   string   `dynamodbav:"programaId"`
	Integrantes  []string `dynamodbav:"integrantes"`
	OrientadorID string   `dynamodbav:"orientadorId"`
}

type itemVinculo struct {
	PK        string `dynamodbav:"PK"`
	ProjetoID string `dynamodbav:"projetoId"`
}

func chaveProjeto(id string) common.Item { return common.Chave("PROJECT#"+id, "PROFILE") }

func chaveVinculo(rgm string) common.Item { return common.Chave("MEMBRO#"+rgm, "PROJETO") }

// criarVinculo entra na transação que põe o RGM no grupo.
func criarVinculo(rgm, projetoID string) types.TransactWriteItem {
	v := chaveVinculo(rgm)
	v["projetoId"] = common.S(projetoID)
	return common.Put(v, "attribute_not_exists(PK)", nil)
}

// apagarVinculo entra na transação que tira o RGM do grupo. Só apaga se o
// vínculo for deste projeto (ou não existir).
func apagarVinculo(rgm, projetoID string) types.TransactWriteItem {
	return common.Delete(chaveVinculo(rgm),
		"attribute_not_exists(PK) OR projetoId = :projeto", common.Item{":projeto": common.S(projetoID)})
}

func listaDeRGMs(rgms []string) *types.AttributeValueMemberL {
	valores := make([]types.AttributeValue, 0, len(rgms))
	for _, r := range rgms {
		valores = append(valores, common.S(r))
	}
	return &types.AttributeValueMemberL{Value: valores}
}

func toProjeto(it item) Projeto {
	// Projeto gravado sem integrantes vira null no JSON e quebra o front
	// (p.integrantes.includes). Sempre devolve lista.
	if it.Integrantes == nil {
		it.Integrantes = []string{}
	}
	return Projeto{
		ID:           it.PK[len("PROJECT#"):],
		Nome:         it.Nome,
		Descricao:    it.Descricao,
		ProgramaID:   it.ProgramaID,
		Integrantes:  it.Integrantes,
		OrientadorID: it.OrientadorID,
	}
}

// salvar cria o projeto. Os integrantes iniciais (o criador, no /meu-pfc)
// ganham o vínculo na mesma transação: errAlunoJaTemProjeto se algum deles
// já estiver em outro projeto.
func salvar(ctx context.Context, programaID string, novo NovoProjeto) (Projeto, error) {
	id := uuid.NewString()
	if novo.Integrantes == nil {
		// nil vira NULL no DynamoDB; lista vazia vira L — é o que
		// list_append e size() esperam.
		novo.Integrantes = []string{}
	}

	it := item{
		PK:          "PROJECT#" + id,
		SK:          "PROFILE",
		GSI1PK:      "PROGRAM#" + programaID,
		GSI1SK:      "PROJECT#" + id,
		Nome:        novo.Nome,
		Descricao:   novo.Descricao,
		ProgramaID:  programaID,
		Integrantes: novo.Integrantes,
	}
	av, err := attributevalue.MarshalMap(it)
	if err != nil {
		return Projeto{}, fmt.Errorf("falha ao serializar projeto: %w", err)
	}

	escritas := []types.TransactWriteItem{common.Put(av, "attribute_not_exists(PK)", nil)}
	for _, rgm := range novo.Integrantes {
		escritas = append(escritas, criarVinculo(rgm, id))
	}
	err = common.Transacao(ctx, escritas...)
	if err != nil {
		var c *common.ErroDeCondicao
		if errors.As(err, &c) {
			return Projeto{}, errAlunoJaTemProjeto
		}
		return Projeto{}, err
	}
	return toProjeto(it), nil
}

// Buscar devolve o projeto pelo id. ok=false quando ele não existe.
func Buscar(ctx context.Context, projetoID string) (p Projeto, ok bool, err error) {
	it, ok, err := common.Buscar[item](ctx, chaveProjeto(projetoID))
	if err != nil || !ok {
		return Projeto{}, ok, err
	}
	return toProjeto(it), true, nil
}

// buscarPorRGM acha o projeto em que o RGM é integrante, pelo vínculo.
// ok=false se não houver.
func buscarPorRGM(ctx context.Context, rgm string) (p Projeto, ok bool, err error) {
	v, ok, err := common.Buscar[itemVinculo](ctx, chaveVinculo(rgm), "projetoId")
	if err != nil || !ok {
		return Projeto{}, false, err
	}
	return Buscar(ctx, v.ProjetoID)
}

// projetosDosRGMs devolve, numa única ida ao banco, o projeto de cada RGM que
// já está em algum (rgm → projetoId). RGM livre fica fora do mapa.
func projetosDosRGMs(ctx context.Context, rgms []string) (map[string]string, error) {
	chaves := make([]common.Item, 0, len(rgms))
	for _, rgm := range rgms {
		chaves = append(chaves, chaveVinculo(rgm))
	}
	itens, err := common.BuscarVarios(ctx, chaves, "PK", "projetoId")
	if err != nil {
		return nil, err
	}
	ocupados := make(map[string]string, len(itens))
	for _, it := range itens {
		ocupados[strings.TrimPrefix(common.Texto(it, "PK"), "MEMBRO#")] = common.Texto(it, "projetoId")
	}
	return ocupados, nil
}

func listarPorPrograma(ctx context.Context, programaID string) ([]Projeto, error) {
	itens, err := common.QueryNoIndice[item](ctx, "PROGRAM#"+programaID)
	if err != nil {
		return nil, err
	}
	projetos := make([]Projeto, 0, len(itens))
	for _, it := range itens {
		projetos = append(projetos, toProjeto(it))
	}
	return projetos, nil
}

func atualizarCampos(ctx context.Context, projetoID, expressao string, valores common.Item) error {
	in := &dynamodb.UpdateItemInput{
		TableName:           aws.String(common.Tabela),
		Key:                 chaveProjeto(projetoID),
		UpdateExpression:    aws.String(expressao),
		ConditionExpression: aws.String("attribute_exists(PK)"),
	}
	if len(valores) > 0 {
		in.ExpressionAttributeValues = valores
	}
	_, err := common.DynamoDB().UpdateItem(ctx, in)
	return err
}

func associarOrientador(ctx context.Context, projetoID, orientadorID string) error {
	return atualizarCampos(ctx, projetoID, "SET orientadorId = :o", common.Item{":o": common.S(orientadorID)})
}

func removerOrientador(ctx context.Context, projetoID string) error {
	return atualizarCampos(ctx, projetoID, "REMOVE orientadorId", nil)
}

func atualizarProjeto(ctx context.Context, projetoID string, dados AtualizarProjeto) error {
	return atualizarCampos(ctx, projetoID, "SET nome = :n, descricao = :d",
		common.Item{":n": common.S(dados.Nome), ":d": common.S(dados.Descricao)})
}

// adicionarIntegrante põe o RGM no grupo e cria o vínculo dele, numa
// transação. `atual` é a lista lida pelo chamador: vazia, grava a lista nova
// (o atributo pode estar ausente ou NULL em projetos antigos, e list_append
// falha em NULL); senão, acrescenta. A condição barra RGM repetido e grupo
// cheio; o vínculo barra aluno que já está em outro projeto.
//
// Erros: errGrupoMudou (condição do projeto) e errAlunoJaTemProjeto.
func adicionarIntegrante(ctx context.Context, projetoID, rgm string, atual []string) error {
	novo := listaDeRGMs([]string{rgm})
	var grupo types.TransactWriteItem
	if len(atual) == 0 {
		grupo = common.Update(chaveProjeto(projetoID), "SET integrantes = :novo",
			"attribute_exists(PK) AND (attribute_not_exists(integrantes) OR attribute_type(integrantes, :nulo) OR size(integrantes) = :zero)",
			common.Item{":novo": novo, ":nulo": common.S("NULL"), ":zero": common.N(0)})
	} else {
		grupo = common.Update(chaveProjeto(projetoID), "SET integrantes = list_append(integrantes, :novo)",
			"attribute_exists(PK) AND NOT contains(integrantes, :rgm) AND size(integrantes) < :max",
			common.Item{":novo": novo, ":rgm": common.S(rgm), ":max": common.N(maxIntegrantes)})
	}
	err := common.Transacao(ctx, grupo, criarVinculo(rgm, projetoID))
	switch {
	case common.FalhouItem(err, 1):
		return errAlunoJaTemProjeto
	case common.FalhouItem(err, 0):
		return errGrupoMudou
	}
	return err
}

func removerIntegrante(ctx context.Context, projetoID, rgm string) error {
	it, ok, err := common.Buscar[item](ctx, chaveProjeto(projetoID))
	if err != nil {
		return err
	}
	if !ok {
		return errProjetoNaoExiste
	}

	restantes := make([]string, 0, len(it.Integrantes))
	for _, r := range it.Integrantes {
		if r != rgm {
			restantes = append(restantes, r)
		}
	}
	if len(restantes) == len(it.Integrantes) {
		return errNaoIntegrante
	}

	// A condição garante que a lista não mudou entre a leitura e a escrita
	// (senão a entrada de outro aluno no meio seria apagada) e que o projeto
	// não foi excluído — sem ela, o update recriaria um item só com PK/SK e
	// integrantes.
	err = common.Transacao(ctx,
		common.Update(chaveProjeto(projetoID), "SET integrantes = :lista",
			"attribute_exists(PK) AND integrantes = :anterior",
			common.Item{":lista": listaDeRGMs(restantes), ":anterior": listaDeRGMs(it.Integrantes)}),
		apagarVinculo(rgm, projetoID),
	)
	var c *common.ErroDeCondicao
	if errors.As(err, &c) {
		return errGrupoMudou
	}
	return err
}

// deletarProjeto apaga o projeto, os vínculos dos integrantes e tudo o que
// mora na partição dele (convites, entregas e referências). Sem a cascata,
// o aluno continuava vendo convite de grupo inexistente e as entregas
// ficavam órfãs no GSI da atividade.
func deletarProjeto(ctx context.Context, projetoID string) error {
	it, ok, err := common.Buscar[item](ctx, chaveProjeto(projetoID), "PK", "integrantes")
	if err != nil {
		return err
	}
	if !ok {
		return errProjetoNaoExiste
	}

	escritas := []types.TransactWriteItem{
		common.Delete(chaveProjeto(projetoID), "attribute_exists(PK) AND integrantes = :lista",
			common.Item{":lista": listaDeRGMs(it.Integrantes)}),
	}
	if it.Integrantes == nil {
		// Projeto antigo sem a lista: a comparação acima nunca bateria.
		escritas[0] = common.Delete(chaveProjeto(projetoID), "attribute_exists(PK)", nil)
	}
	for _, rgm := range it.Integrantes {
		escritas = append(escritas, apagarVinculo(rgm, projetoID))
	}
	if err := common.Transacao(ctx, escritas...); err != nil {
		if common.FalhouItem(err, 0) {
			return errGrupoMudou
		}
		return err
	}

	itens, err := common.QueryTudo(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(common.Tabela),
		KeyConditionExpression:    aws.String("PK = :pk"),
		ExpressionAttributeValues: common.Item{":pk": common.S("PROJECT#" + projetoID)},
		ProjectionExpression:      aws.String("PK, SK"),
	})
	if err != nil {
		return fmt.Errorf("projeto apagado, mas falhou ao listar os itens dele: %w", err)
	}
	chaves := make([]common.Item, 0, len(itens))
	for _, i := range itens {
		chaves = append(chaves, common.Item{"PK": i["PK"], "SK": i["SK"]})
	}
	if err := common.ApagarChaves(ctx, chaves); err != nil {
		return fmt.Errorf("projeto apagado, mas falhou ao apagar os itens dele: %w", err)
	}
	return nil
}
