// Command migrar prepares a table created before the GSI/lock model (see the
// README, "Modelagem no DynamoDB"). It is idempotent; without -aplicar it
// only reports what it would do. Conflicts are listed for manual repair,
// since the new uniqueness rules would reject them.
//
//	TABLE_NAME=... go run ./cmd/migrar
//	TABLE_NAME=... go run ./cmd/migrar -aplicar
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

var apply = flag.Bool("aplicar", false, "grava as mudanças (sem a flag, só simula)")

type migration struct {
	table                     *dynamo.Table
	indexed, locks, conflicts int
	// planned holds the locks of this run (PK|SK → owner), so a dry run
	// also detects conflicts between items it has not written.
	planned map[string]string
}

func main() {
	flag.Parse()
	name := os.Getenv("TABLE_NAME")
	if name == "" {
		log.Fatal("defina TABLE_NAME")
	}
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}
	m := migration{table: &dynamo.Table{Client: dynamodb.NewFromConfig(cfg), Name: name}, planned: map[string]string{}}

	items, err := m.scan(ctx)
	if err != nil {
		log.Fatalf("scan: %v", err)
	}
	for _, it := range items {
		m.migrate(ctx, it)
	}

	mode := "simulação (use -aplicar para gravar)"
	if *apply {
		mode = "aplicado"
	}
	fmt.Printf("%s: %d itens indexados no GSI1, %d travas/vínculos criados, %d conflitos\n",
		mode, m.indexed, m.locks, m.conflicts)
}

func (m *migration) scan(ctx context.Context) ([]dynamo.Item, error) {
	var items []dynamo.Item
	in := &dynamodb.ScanInput{TableName: aws.String(m.table.Name)}
	for {
		out, err := m.table.Client.Scan(ctx, in)
		if err != nil {
			return nil, err
		}
		items = append(items, out.Items...)
		if len(out.LastEvaluatedKey) == 0 {
			return items, nil
		}
		in.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func (m *migration) migrate(ctx context.Context, it dynamo.Item) {
	if dynamo.String(it, "SK") != "PROFILE" {
		return
	}
	pk := dynamo.String(it, "PK")
	switch {
	case strings.HasPrefix(pk, "CURSO#"):
		m.index(ctx, it, "TURMAS")
		lock := dynamo.Key(fmt.Sprintf("TURMA_UNICA#%s#%s#%s",
			dynamo.String(it, "nome"), dynamo.String(it, "turno"), dynamo.String(it, "periodo")), "TURMA_UNICA")
		lock["cursoId"] = dynamo.S(strings.TrimPrefix(pk, "CURSO#"))
		m.lock(ctx, lock, "cursoId", "turma duplicada "+pk)
	case strings.HasPrefix(pk, "PROGRAM#"):
		m.index(ctx, it, "PROGRAMAS")
		link := dynamo.Key("CURSO#"+dynamo.String(it, "cursoId"), "PROGRAMA")
		link["programaId"] = dynamo.S(strings.TrimPrefix(pk, "PROGRAM#"))
		m.lock(ctx, link, "programaId", "segundo programa na turma "+dynamo.String(it, "cursoId"))
	case strings.HasPrefix(pk, "STUDENT#"):
		m.index(ctx, it, "MATRICULAS")
	case strings.HasPrefix(pk, "PROJECT#"):
		members, _ := it["integrantes"].(*types.AttributeValueMemberL)
		if members == nil {
			return
		}
		for _, v := range members.Value {
			rgm, _ := v.(*types.AttributeValueMemberS)
			if rgm == nil {
				continue
			}
			link := dynamo.Key("MEMBRO#"+rgm.Value, "PROJETO")
			link["projetoId"] = dynamo.S(strings.TrimPrefix(pk, "PROJECT#"))
			m.lock(ctx, link, "projetoId", "RGM "+rgm.Value+" em mais de um projeto")
		}
	}
}

func (m *migration) index(ctx context.Context, it dynamo.Item, partition string) {
	if dynamo.String(it, "GSI1PK") != "" {
		return
	}
	m.indexed++
	if !*apply {
		return
	}
	pk := dynamo.String(it, "PK")
	_, err := m.table.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(m.table.Name),
		Key:                       dynamo.Key(pk, "PROFILE"),
		UpdateExpression:          aws.String("SET GSI1PK = :p, GSI1SK = :s"),
		ConditionExpression:       aws.String("attribute_exists(PK)"),
		ExpressionAttributeValues: dynamo.Item{":p": dynamo.S(partition), ":s": dynamo.S(pk)},
	})
	if err != nil {
		log.Fatalf("index %s: %v", pk, err)
	}
}

// lock creates the lock/link item unless it exists. An existing one owned
// by another item is a conflict; owned by the same one, already migrated.
func (m *migration) lock(ctx context.Context, item dynamo.Item, ownerAttr, conflict string) {
	owner := dynamo.String(item, ownerAttr)
	key := dynamo.String(item, "PK") + "|" + dynamo.String(item, "SK")
	if planned, ok := m.planned[key]; ok {
		if planned != owner {
			m.conflict(conflict)
		}
		return
	}
	m.planned[key] = owner

	current, found, err := dynamo.Get[dynamo.Item](ctx, m.table, dynamo.Item{"PK": item["PK"], "SK": item["SK"]})
	if err != nil {
		log.Fatalf("read %s: %v", key, err)
	}
	if found {
		if dynamo.String(current, ownerAttr) != owner {
			m.conflict(conflict)
		}
		return
	}
	m.locks++
	if !*apply {
		return
	}
	_, err = m.table.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(m.table.Name),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		log.Fatalf("write %s: %v", key, err)
	}
}

func (m *migration) conflict(msg string) {
	m.conflicts++
	fmt.Println("CONFLITO:", msg)
}
