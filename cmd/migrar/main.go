// Comando migrar prepara uma tabela já existente para a modelagem com GSI,
// travas e vínculos (ver README, seção "Modelagem no DynamoDB"):
//
//   - turma, programa e matrícula ganham GSI1PK/GSI1SK (a listagem deixou de
//     ser Scan);
//   - cada turma ganha a trava TURMA_UNICA#<nome>#<turno>#<período>;
//   - cada programa ganha o vínculo CURSO#<cursoId>/PROGRAMA;
//   - cada integrante de projeto ganha o vínculo MEMBRO#<rgm>/PROJETO.
//
// É idempotente: rodar de novo não muda nada. Sem -aplicar só mostra o que
// faria. Conflitos (duas turmas iguais, dois programas na mesma turma, um
// aluno em dois projetos) são listados e ficam para correção manual — com
// eles a regra nova recusaria a gravação.
//
//	TABLE_NAME=... go run ./cmd/migrar            # simulação
//	TABLE_NAME=... go run ./cmd/migrar -aplicar
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

var aplicar = flag.Bool("aplicar", false, "grava as mudanças (sem a flag, só simula)")

type contagem struct {
	gsi, travas, conflitos int
	// planejadas guarda as travas desta execução (PK|SK → dono), pra que a
	// simulação também ache conflito entre itens que ainda não gravou.
	planejadas map[string]string
}

func main() {
	flag.Parse()
	if common.Tabela == "" {
		log.Fatal("defina TABLE_NAME")
	}
	ctx := context.Background()

	var itens []common.Item
	in := &dynamodb.ScanInput{TableName: aws.String(common.Tabela)}
	for {
		out, err := common.DynamoDB().Scan(ctx, in)
		if err != nil {
			log.Fatalf("scan: %v", err)
		}
		itens = append(itens, out.Items...)
		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		in.ExclusiveStartKey = out.LastEvaluatedKey
	}

	c := contagem{planejadas: map[string]string{}}
	for _, it := range itens {
		if common.Texto(it, "SK") != "PROFILE" {
			continue
		}
		pk := common.Texto(it, "PK")
		switch {
		case strings.HasPrefix(pk, "CURSO#"):
			c.indexar(ctx, it, "TURMAS")
			trava := common.Chave(fmt.Sprintf("TURMA_UNICA#%s#%s#%s",
				common.Texto(it, "nome"), common.Texto(it, "turno"), common.Texto(it, "periodo")), "TURMA_UNICA")
			trava["cursoId"] = common.S(strings.TrimPrefix(pk, "CURSO#"))
			c.travar(ctx, trava, "cursoId", "turma duplicada "+pk)
		case strings.HasPrefix(pk, "PROGRAM#"):
			c.indexar(ctx, it, "PROGRAMAS")
			v := common.Chave("CURSO#"+common.Texto(it, "cursoId"), "PROGRAMA")
			v["programaId"] = common.S(strings.TrimPrefix(pk, "PROGRAM#"))
			c.travar(ctx, v, "programaId", "segundo programa na turma "+common.Texto(it, "cursoId"))
		case strings.HasPrefix(pk, "STUDENT#"):
			c.indexar(ctx, it, "MATRICULAS")
		case strings.HasPrefix(pk, "PROJECT#"):
			lista, _ := it["integrantes"].(*types.AttributeValueMemberL)
			if lista == nil {
				continue
			}
			for _, r := range lista.Value {
				rgm, _ := r.(*types.AttributeValueMemberS)
				if rgm == nil {
					continue
				}
				v := common.Chave("MEMBRO#"+rgm.Value, "PROJETO")
				v["projetoId"] = common.S(strings.TrimPrefix(pk, "PROJECT#"))
				c.travar(ctx, v, "projetoId", "RGM "+rgm.Value+" em mais de um projeto")
			}
		}
	}

	modo := "simulação (use -aplicar para gravar)"
	if *aplicar {
		modo = "aplicado"
	}
	fmt.Printf("%s: %d itens indexados no GSI1, %d travas/vínculos criados, %d conflitos\n",
		modo, c.gsi, c.travas, c.conflitos)
}

// indexar põe GSI1PK/GSI1SK no item, se ainda não tiver.
func (c *contagem) indexar(ctx context.Context, it common.Item, particao string) {
	if common.Texto(it, "GSI1PK") != "" {
		return
	}
	c.gsi++
	if !*aplicar {
		return
	}
	pk := common.Texto(it, "PK")
	_, err := common.DynamoDB().UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(common.Tabela),
		Key:                       common.Chave(pk, "PROFILE"),
		UpdateExpression:          aws.String("SET GSI1PK = :p, GSI1SK = :s"),
		ConditionExpression:       aws.String("attribute_exists(PK)"),
		ExpressionAttributeValues: common.Item{":p": common.S(particao), ":s": common.S(pk)},
	})
	if err != nil {
		log.Fatalf("indexar %s: %v", pk, err)
	}
}

// travar cria a trava/vínculo se não existir. Se já existe apontando para
// outro item, é conflito; apontando para o mesmo, já foi migrado.
func (c *contagem) travar(ctx context.Context, item common.Item, campo, conflito string) {
	chave := common.Texto(item, "PK") + "|" + common.Texto(item, "SK")
	if dono, ok := c.planejadas[chave]; ok {
		if dono != common.Texto(item, campo) {
			c.conflitos++
			fmt.Println("CONFLITO:", conflito)
		}
		return
	}
	c.planejadas[chave] = common.Texto(item, campo)

	atual, err := common.DynamoDB().GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(common.Tabela),
		Key:       common.Item{"PK": item["PK"], "SK": item["SK"]},
	})
	if err != nil {
		log.Fatalf("ler %s: %v", common.Texto(item, "PK"), err)
	}
	if atual.Item != nil {
		if common.Texto(atual.Item, campo) != common.Texto(item, campo) {
			c.conflitos++
			fmt.Println("CONFLITO:", conflito)
		}
		return
	}
	c.travas++
	if !*aplicar {
		return
	}
	_, err = common.DynamoDB().PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(common.Tabela),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		log.Fatalf("gravar %s: %v", common.Texto(item, "PK"), err)
	}
}
