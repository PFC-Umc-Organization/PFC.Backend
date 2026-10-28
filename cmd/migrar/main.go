// Command migrar brings a table written by older versions to the current
// model (see the README, "Modelagem no DynamoDB"): GSI on cohorts, programs
// and enrollments (these by cohort), the uniqueness locks and links, and
// courses for cohorts that predate them. It is idempotent; without -aplicar
// it only reports what it would do. Conflicts are listed for manual repair,
// since the uniqueness rules would reject them. With COGNITO_USER_POOL_ID it
// also copies the accounts' names from Cognito into the table.
//
//	TABLE_NAME=... COGNITO_USER_POOL_ID=... go run ./cmd/migrar
//	TABLE_NAME=... COGNITO_USER_POOL_ID=... go run ./cmd/migrar -aplicar
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/academic"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/identity"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

var apply = flag.Bool("aplicar", false, "grava as mudanças (sem a flag, só simula)")

type migration struct {
	table                                      *dynamo.Table
	indexed, locks, courses, linked, conflicts int
	// planned holds the locks of this run (PK|SK → owner) and plannedCourses
	// the courses it creates (lower-case name → id), so a dry run also sees
	// what it has not written.
	planned        map[string]string
	plannedCourses map[string]string
	cohorts        map[string]bool
	// spellings counts, per lower-case course name, how cohorts write it;
	// a new course takes the most common one.
	spellings map[string]map[string]int
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
	m := migration{
		table:          &dynamo.Table{Client: dynamodb.NewFromConfig(cfg), Name: name},
		planned:        map[string]string{},
		plannedCourses: map[string]string{},
		cohorts:        map[string]bool{},
		spellings:      map[string]map[string]int{},
	}

	items, err := m.scan(ctx)
	if err != nil {
		log.Fatalf("scan: %v", err)
	}
	for _, it := range items {
		if pk := dynamo.String(it, "PK"); strings.HasPrefix(pk, "CURSO#") && dynamo.String(it, "SK") == "PROFILE" {
			m.cohorts[strings.TrimPrefix(pk, "CURSO#")] = true
			name := strings.Join(strings.Fields(dynamo.String(it, "nome")), " ")
			lower := strings.ToLower(name)
			if m.spellings[lower] == nil {
				m.spellings[lower] = map[string]int{}
			}
			m.spellings[lower][name]++
		}
	}
	for _, it := range items {
		m.migrate(ctx, it)
	}

	mode := "simulação (use -aplicar para gravar)"
	if *apply {
		mode = "aplicado"
	}
	fmt.Printf("%s: %d itens indexados no GSI1, %d travas/vínculos criados, %d cursos criados, %d turmas ligadas ao curso, %d conflitos\n",
		mode, m.indexed, m.locks, m.courses, m.linked, m.conflicts)

	if pool := os.Getenv("COGNITO_USER_POOL_ID"); pool != "" {
		cognito := identity.NewCognito(cognitoidentityprovider.NewFromConfig(cfg), "", pool)
		students, staff, err := identity.SyncNames(ctx, cognito, identity.NewDynamoProfiles(m.table), *apply)
		if err != nil {
			log.Fatalf("nomes: %v", err)
		}
		fmt.Printf("%s: nomes de %d alunos e %d contas da equipe copiados do Cognito\n", mode, students, staff)
	}
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
		if term := dynamo.String(it, "periodo"); !termFormat.MatchString(term) {
			fmt.Printf("AVISO: turma %s com período fora do formato AAAA.S: %q\n", pk, term)
		}
		courseID := dynamo.String(it, "graduacaoId")
		if courseID == "" {
			m.linkCourse(ctx, it)
			return
		}
		lock := dynamo.Key(academic.CohortLockPK(courseID, dynamo.String(it, "turno"), dynamo.String(it, "periodo")), "TURMA_UNICA")
		lock["cursoId"] = dynamo.S(strings.TrimPrefix(pk, "CURSO#"))
		m.lock(ctx, lock, "cursoId", "turma duplicada "+pk)
	case strings.HasPrefix(pk, "PROGRAM#"):
		m.index(ctx, it, "PROGRAMAS")
		if cohort := dynamo.String(it, "cursoId"); !m.cohorts[cohort] {
			fmt.Printf("AVISO: programa %s aponta para a turma %s, que não existe\n", pk, cohort)
		}
		link := dynamo.Key("CURSO#"+dynamo.String(it, "cursoId"), "PROGRAMA")
		link["programaId"] = dynamo.S(strings.TrimPrefix(pk, "PROGRAM#"))
		m.lock(ctx, link, "programaId", "segundo programa na turma "+dynamo.String(it, "cursoId"))
	case strings.HasPrefix(pk, "STUDENT#"):
		cohort := dynamo.String(it, "turmaId")
		if cohort == "" {
			fmt.Printf("AVISO: matrícula %s sem turma; fica fora das listas até ser refeita\n", pk)
			return
		}
		m.index(ctx, it, academic.EnrollmentsPartition(cohort))
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
	if dynamo.String(it, "GSI1PK") == partition {
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

	current, found, err := m.get(ctx, dynamo.Item{"PK": item["PK"], "SK": item["SK"]})
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

var termFormat = regexp.MustCompile(`^\d{4}\.[12]$`)

// linkCourse points a cohort written before courses existed to its course,
// creating the course from the cohort's name when needed, and swaps its
// name-keyed lock for the id-keyed one, counting the cohort on the course.
func (m *migration) linkCourse(ctx context.Context, it dynamo.Item) {
	pk := dynamo.String(it, "PK")
	cohortID := strings.TrimPrefix(pk, "CURSO#")
	stored := dynamo.String(it, "nome")
	name := strings.Join(strings.Fields(stored), " ")
	shift, term := dynamo.String(it, "turno"), dynamo.String(it, "periodo")
	if name == "" {
		m.conflict("turma " + pk + " sem nome de curso")
		return
	}

	courseID, err := m.course(ctx, name)
	if err != nil {
		log.Fatalf("course %q: %v", name, err)
	}
	newLock := academic.CohortLockPK(courseID, shift, term)
	if owner, ok := m.planned[newLock+"|TURMA_UNICA"]; ok && owner != cohortID {
		m.conflict("turma duplicada " + pk)
		return
	}
	m.planned[newLock+"|TURMA_UNICA"] = cohortID
	m.linked++
	if !*apply {
		return
	}

	lock := dynamo.Key(newLock, "TURMA_UNICA")
	lock["cursoId"] = dynamo.S(cohortID)
	owned := "attribute_not_exists(PK) OR cursoId = :id"
	err = m.table.Transact(ctx,
		m.table.Update(dynamo.Key(pk, "PROFILE"), "SET graduacaoId = :c REMOVE nome", "attribute_not_exists(graduacaoId)",
			dynamo.Item{":c": dynamo.S(courseID)}),
		m.table.Delete(dynamo.Key(fmt.Sprintf("TURMA_UNICA#%s#%s#%s", stored, shift, term), "TURMA_UNICA"), owned,
			dynamo.Item{":id": dynamo.S(cohortID)}),
		m.table.Put(lock, owned, dynamo.Item{":id": dynamo.S(cohortID)}),
		m.table.Update(dynamo.Key("COURSE#"+courseID, "PROFILE"), "SET turmas = if_not_exists(turmas, :zero) + :one",
			"attribute_exists(PK)", dynamo.Item{":zero": dynamo.N(0), ":one": dynamo.N(1)}),
	)
	if dynamo.FailedAt(err, 2) {
		m.linked--
		m.conflict("turma duplicada " + pk)
		return
	}
	if err != nil {
		log.Fatalf("link %s: %v", pk, err)
	}
}

// course returns the id of the course with this name (ignoring case),
// creating it when no course has it.
func (m *migration) course(ctx context.Context, name string) (string, error) {
	lower := strings.ToLower(name)
	if id, ok := m.plannedCourses[lower]; ok {
		return id, nil
	}
	lockKey := dynamo.Key("CURSO_NOME#"+lower, "CURSO_NOME")
	lock, found, err := m.get(ctx, lockKey)
	if err != nil {
		return "", err
	}
	if found {
		m.plannedCourses[lower] = dynamo.String(lock, "cursoId")
		return m.plannedCourses[lower], nil
	}

	name = m.spelling(lower, name)
	id := uuid.NewString()
	m.plannedCourses[lower] = id
	m.courses++
	fmt.Printf("curso novo: %q (a partir das turmas)\n", name)
	if !*apply {
		return id, nil
	}
	lockKey["cursoId"] = dynamo.S(id)
	lockKey["nome"] = dynamo.S(name)
	return id, m.table.Transact(ctx,
		m.table.Put(dynamo.Item{
			"PK": dynamo.S("COURSE#" + id), "SK": dynamo.S("PROFILE"),
			"GSI1PK": dynamo.S("COURSES"), "GSI1SK": dynamo.S("COURSE#" + id),
			"nome": dynamo.S(name), "turmas": dynamo.N(0),
		}, "attribute_not_exists(PK)", nil),
		m.table.Put(lockKey, "attribute_not_exists(PK)", nil),
	)
}

// spelling picks the most common way cohorts write the course name; on a tie,
// the one with more capitals ("Engenharia de Software" over "engenharia de
// software"), then the first in alphabetical order.
func (m *migration) spelling(lower, fallback string) string {
	best, bestCount := fallback, 0
	for name, n := range m.spellings[lower] {
		better := n > bestCount ||
			n == bestCount && (capitals(name) > capitals(best) || capitals(name) == capitals(best) && name < best)
		if better {
			best, bestCount = name, n
		}
	}
	return best
}

func capitals(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsUpper(r) {
			n++
		}
	}
	return n
}

func (m *migration) get(ctx context.Context, key dynamo.Item) (dynamo.Item, bool, error) {
	out, err := m.table.Client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(m.table.Name), Key: key})
	if err != nil {
		return nil, false, err
	}
	return out.Item, out.Item != nil, nil
}
