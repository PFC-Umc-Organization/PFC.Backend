package matricula

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// Modelagem:
//
//	matrícula  PK=STUDENT#<rgm>  SK=PROFILE  GSI1PK=MATRICULAS  GSI1SK=STUDENT#<rgm>  status, turmaId
//
// O GSI1 lista as matrículas sem Scan. O Pre Sign-up Lambda (fora deste
// repo) lê este item pela chave — PK e SK não podem mudar.

func chave(rgm string) common.Item { return common.Chave("STUDENT#"+rgm, "PROFILE") }

const batchSize = common.TamanhoLote

func gravarRGMs(ctx context.Context, rgms []string, turmaID string) []RGMFalha {
	var falhas []RGMFalha

	for i := 0; i < len(rgms); i += batchSize {
		chunk := rgms[i:min(i+batchSize, len(rgms))]

		writes := make([]types.WriteRequest, 0, len(chunk))
		for _, rgm := range chunk {
			writes = append(writes, types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: common.Item{
						"PK":      common.S("STUDENT#" + rgm),
						"SK":      common.S("PROFILE"),
						"GSI1PK":  common.S("MATRICULAS"),
						"GSI1SK":  common.S("STUDENT#" + rgm),
						"status":  common.S("ACTIVE"),
						"turmaId": common.S(turmaID),
					},
				},
			})
		}

		if err := common.GravarLote(ctx, writes); err != nil {
			for _, rgm := range chunk {
				falhas = append(falhas, RGMFalha{RGM: rgm, Erro: err.Error()})
			}
		}
	}

	return falhas
}

type item struct {
	PK      string `dynamodbav:"PK"`
	Status  string `dynamodbav:"status"`
	TurmaID string `dynamodbav:"turmaId"`
}

func listarRGMs(ctx context.Context) ([]Matricula, error) {
	itens, err := common.QueryNoIndice[item](ctx, "MATRICULAS")
	if err != nil {
		return nil, err
	}
	matriculas := make([]Matricula, 0, len(itens))
	for _, it := range itens {
		status := "INATIVO"
		if it.Status == "ACTIVE" {
			status = "ATIVO"
		}
		matriculas = append(matriculas, Matricula{
			RGM:     strings.TrimPrefix(it.PK, "STUDENT#"),
			Status:  status,
			TurmaID: it.TurmaID,
		})
	}
	return matriculas, nil
}

// TurmaDoRGM devolve a turma vinculada ao RGM pré-autorizado, ou "" se o
// RGM não está na allowlist (ou foi pré-autorizado antes desta turmaId
// existir). Usado por `auth` e `usuario` pra preencher CursoIds do aluno.
func TurmaDoRGM(ctx context.Context, rgm string) (string, error) {
	if rgm == "" {
		return "", nil
	}
	it, _, err := common.Buscar[item](ctx, chave(rgm), "turmaId")
	return it.TurmaID, err
}

// TurmasDosRGMs faz o mesmo que TurmaDoRGM pra vários RGMs numa única ida ao
// banco (BatchGetItem). RGM fora da allowlist fica fora do mapa.
func TurmasDosRGMs(ctx context.Context, rgms []string) (map[string]string, error) {
	chaves := make([]common.Item, 0, len(rgms))
	for _, rgm := range rgms {
		chaves = append(chaves, chave(rgm))
	}
	itens, err := common.BuscarVarios(ctx, chaves, "PK", "turmaId")
	if err != nil {
		return nil, err
	}
	turmas := make(map[string]string, len(itens))
	for _, it := range itens {
		if t := common.Texto(it, "turmaId"); t != "" {
			turmas[strings.TrimPrefix(common.Texto(it, "PK"), "STUDENT#")] = t
		}
	}
	return turmas, nil
}

// Existe diz se o RGM está pré-autorizado.
func Existe(ctx context.Context, rgm string) (bool, error) {
	return common.Existe(ctx, chave(rgm))
}

func removerRGMs(ctx context.Context, rgms []string) []RGMFalha {
	var falhas []RGMFalha

	for i := 0; i < len(rgms); i += batchSize {
		chunk := rgms[i:min(i+batchSize, len(rgms))]

		writes := make([]types.WriteRequest, 0, len(chunk))
		for _, rgm := range chunk {
			writes = append(writes, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: chave(rgm),
				},
			})
		}

		if err := common.GravarLote(ctx, writes); err != nil {
			for _, rgm := range chunk {
				falhas = append(falhas, RGMFalha{RGM: rgm, Erro: err.Error()})
			}
		}
	}

	return falhas
}
