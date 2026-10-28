// Package integracao testa a API de ponta a ponta: DynamoDB Local de verdade,
// Cognito e APIs externas falsos. Rodar com scripts/test-integration.sh.
package integracao

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/app"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/httpapi"
)

var (
	r       *httpapi.Router
	table   *dynamo.Table
	cognito = newFakeCognito()
	bucket  = newFakeS3()
)

func TestMain(m *testing.M) {
	required := []string{
		"AWS_ENDPOINT_URL_DYNAMODB", "AWS_ENDPOINT_URL_COGNITO_IDENTITY_PROVIDER",
		"REFERENCIAS_CROSSREF_URL", "REFERENCIAS_OPENALEX_URL", "AWS_ENDPOINT_URL_S3", "UPLOADS_BUCKET",
	}
	for _, v := range required {
		if os.Getenv(v) == "" {
			fmt.Println("integracao: " + v + " não definido — pulando (use scripts/test-integration.sh)")
			os.Exit(0)
		}
	}
	if err := cognito.serve(os.Getenv("AWS_ENDPOINT_URL_COGNITO_IDENTITY_PROVIDER")); err != nil {
		fmt.Println("integracao: cognito falso:", err)
		os.Exit(1)
	}
	if err := bucket.serve(os.Getenv("AWS_ENDPOINT_URL_S3")); err != nil {
		fmt.Println("integracao: s3 falso:", err)
		os.Exit(1)
	}
	if err := serveExternalAPIs(os.Getenv("REFERENCIAS_CROSSREF_URL"), os.Getenv("REFERENCIAS_OPENALEX_URL")); err != nil {
		fmt.Println("integracao: apis externas falsas:", err)
		os.Exit(1)
	}
	deps, err := app.Load(context.Background())
	if err != nil {
		fmt.Println("integracao:", err)
		os.Exit(1)
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		fmt.Println("integracao:", err)
		os.Exit(1)
	}
	table = deps.Table
	recorded := deps
	recorded.Table = &dynamo.Table{
		Client: dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) { o.APIOptions = append(o.APIOptions, recorder.middleware("dynamodb")) }),
		Name:   deps.Table.Name, Index: deps.Table.Index,
	}
	recorded.Cognito = cognitoidentityprovider.NewFromConfig(awsCfg, func(o *cognitoidentityprovider.Options) {
		o.APIOptions = append(o.APIOptions, recorder.middleware("cognito-idp"))
	})
	recorded.S3 = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.APIOptions = append(o.APIOptions, recorder.middleware("s3"))
	})
	r = app.New(recorded)
	code := m.Run()
	if err := checkIAMActions(); code == 0 && err != nil {
		fmt.Println(err)
		code = 1
	}
	os.Exit(code)
}

// resetar deixa tabela e Cognito vazios: cada teste começa do zero.
func resetar(t *testing.T) {
	t.Helper()
	if err := criarTabela(context.Background()); err != nil {
		t.Fatal(err)
	}
	cognito.reset()
}

// criarTabela recria a tabela com o mesmo formato da infra: PK/SK e o GSI1
// (GSI1PK/GSI1SK) com projeção ALL.
func criarTabela(ctx context.Context) error {
	ddb := table.Client
	_, _ = ddb.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(table.Name)})
	s := func(n string) types.AttributeDefinition {
		return types.AttributeDefinition{AttributeName: aws.String(n), AttributeType: types.ScalarAttributeTypeS}
	}
	chave := func(pk, sk string) []types.KeySchemaElement {
		return []types.KeySchemaElement{
			{AttributeName: aws.String(pk), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String(sk), KeyType: types.KeyTypeRange},
		}
	}
	_, err := ddb.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:            aws.String(table.Name),
		BillingMode:          types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{s("PK"), s("SK"), s("GSI1PK"), s("GSI1SK")},
		KeySchema:            chave("PK", "SK"),
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{
			IndexName:  aws.String(table.Index),
			KeySchema:  chave("GSI1PK", "GSI1SK"),
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		}},
	})
	return err
}

// criarCursos cadastra os cursos que as turmas do teste usam.
func criarCursos(t *testing.T, nomes ...string) {
	t.Helper()
	for _, n := range nomes {
		esperar(t, admin, "POST", "/cursos", `{"nome":"`+n+`"}`, 201)
	}
}

type quem struct{ perfil, email string }

var (
	admin = quem{"ADMIN", "coord@umc.br"}
	aluno = func(rgm string) quem { return quem{"ALUNO", rgm + "@alunos.umc.br"} }
)

// chamar despacha a requisição pelo router e devolve status e corpo.
func chamar(t *testing.T, q quem, metodo, caminho, corpo string) (int, string) {
	t.Helper()
	req := events.APIGatewayProxyRequest{HTTPMethod: metodo, Path: caminho, Body: corpo}
	if p, query, ok := strings.Cut(caminho, "?"); ok {
		req.Path = p
		valores, err := url.ParseQuery(query)
		if err != nil {
			t.Fatal(err)
		}
		req.QueryStringParameters = map[string]string{}
		for k := range valores {
			req.QueryStringParameters[k] = valores.Get(k)
		}
	}
	// Public routes have no authorizer, as in the API Gateway.
	if q.email != "" {
		req.RequestContext.Authorizer = map[string]interface{}{
			"claims": map[string]interface{}{"custom:perfil": q.perfil, "email": q.email, "sub": cognito.subOf(q.email), "name": q.email},
		}
	}
	resp, err := r.Lambda(context.Background(), req)
	if err != nil {
		t.Fatalf("%s %s: %v", metodo, caminho, err)
	}
	return resp.StatusCode, resp.Body
}

func esperar(t *testing.T, q quem, metodo, caminho, corpo string, status int) string {
	t.Helper()
	got, body := chamar(t, q, metodo, caminho, corpo)
	if got != status {
		t.Fatalf("%s %s = %d, queria %d — %s", metodo, caminho, got, status, body)
	}
	return body
}

func id(t *testing.T, body string) string {
	t.Helper()
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil || v.ID == "" {
		t.Fatalf("resposta sem id: %s", body)
	}
	return v.ID
}

func TestFluxoCompleto(t *testing.T) {
	resetar(t)
	criarCursos(t, "Engenharia de Software", "Sistemas de Informação")
	// --- turmas: unicidade por transação ---
	turmaEng := id(t, esperar(t, admin, "POST", "/turmas", `{"nome":"Engenharia de Software","turno":"NOITE","periodo":"2026.2"}`, 201))
	esperar(t, admin, "POST", "/turmas", `{"nome":"Engenharia de Software","turno":"NOITE","periodo":"2026.2"}`, 409)
	turmaSI := id(t, esperar(t, admin, "POST", "/turmas", `{"nome":"Sistemas de Informação","turno":"NOITE","periodo":"2026.2"}`, 201))
	esperar(t, admin, "PUT", "/turmas/"+turmaSI, `{"nome":"Engenharia de Software","turno":"NOITE","periodo":"2026.2"}`, 409)
	esperar(t, admin, "PUT", "/turmas/"+turmaSI, `{"nome":"Sistemas de Informação","turno":"MANHA","periodo":"2026.2"}`, 200)
	// A combinação antiga ficou livre de novo.
	extra := id(t, esperar(t, admin, "POST", "/turmas", `{"nome":"Sistemas de Informação","turno":"NOITE","periodo":"2026.2"}`, 201))
	esperar(t, admin, "DELETE", "/turmas/"+extra, "", 200)
	esperar(t, admin, "DELETE", "/turmas/inexistente", "", 404)
	if body := esperar(t, admin, "GET", "/turmas", "", 200); strings.Count(body, `"id"`) != 2 {
		t.Fatalf("GET /turmas: queria 2 turmas, veio %s", body)
	}

	// --- programas: um por turma ---
	esperar(t, admin, "POST", "/programas", `{"cursoId":"inexistente"}`, 404)
	prog := id(t, esperar(t, admin, "POST", "/programas", `{"cursoId":"`+turmaEng+`"}`, 201))
	esperar(t, admin, "POST", "/programas", `{"cursoId":"`+turmaEng+`"}`, 409)
	esperar(t, admin, "DELETE", "/turmas/"+turmaEng, "", 409)
	progSI := id(t, esperar(t, admin, "POST", "/programas", `{"cursoId":"`+turmaSI+`"}`, 201))
	esperar(t, admin, "PUT", "/programas/"+progSI, `{"cursoId":"`+turmaEng+`"}`, 409)
	esperar(t, admin, "DELETE", "/programas/"+progSI, "", 200)
	esperar(t, admin, "DELETE", "/turmas/"+turmaSI, "", 200)
	if body := esperar(t, admin, "GET", "/programas", "", 200); strings.Count(body, `"id"`) != 1 {
		t.Fatalf("GET /programas: queria 1 programa, veio %s", body)
	}

	// --- matrículas ---
	esperar(t, admin, "POST", "/admin/students", `{"turmaId":"`+turmaEng+`","rgms":["111","222","333","444","abc"]}`, 200)
	if body := esperar(t, admin, "GET", "/admin/students?turmaId="+turmaEng, "", 200); strings.Count(body, `"rgm"`) != 4 {
		t.Fatalf("GET /admin/students: queria 4 RGMs, veio %s", body)
	}

	// --- projeto do aluno, convite e aceite ---
	p1 := id(t, esperar(t, aluno("111"), "POST", "/meu-pfc", `{"nome":"Athena","integrantes":["222"]}`, 201))
	esperar(t, aluno("111"), "POST", "/meu-pfc", `{"nome":"Outro"}`, 409)
	esperar(t, aluno("999"), "POST", "/meu-pfc", `{"nome":"Sem turma"}`, 403)
	if body := esperar(t, aluno("222"), "GET", "/meus-convites", "", 200); !strings.Contains(body, p1) {
		t.Fatalf("convite não chegou: %s", body)
	}
	esperar(t, aluno("222"), "POST", "/meus-convites/"+p1+"/aceitar", "", 200)

	// --- um projeto por aluno, também pela coordenação ---
	p2 := id(t, esperar(t, admin, "POST", "/programas/"+prog+"/projetos", `{"nome":"Web-Cursos"}`, 201))
	esperar(t, admin, "PUT", "/projetos/"+p2+"/integrantes", `{"rgm":"222"}`, 409)
	esperar(t, admin, "PUT", "/projetos/"+p2+"/integrantes", `{"rgm":"333"}`, 200)
	esperar(t, admin, "PUT", "/projetos/"+p2+"/integrantes", `{"rgm":"333"}`, 409)
	esperar(t, admin, "DELETE", "/projetos/"+p2+"/integrantes", `{"rgm":"333"}`, 200)
	esperar(t, admin, "PUT", "/projetos/"+p1+"/integrantes", `{"rgm":"333"}`, 200)

	body := esperar(t, admin, "GET", "/programas/"+prog+"/projetos", "", 200)
	if !strings.Contains(body, `"integrantes":["111","222","333"]`) || !strings.Contains(body, `"integrantes":[]`) {
		t.Fatalf("integrantes inesperados: %s", body)
	}

	// --- apagar o projeto libera os alunos ---
	esperar(t, admin, "DELETE", "/programas/"+prog, "", 409)
	esperar(t, admin, "DELETE", "/projetos/"+p1, "", 200)
	esperar(t, admin, "DELETE", "/projetos/"+p1, "", 404)
	if body := esperar(t, aluno("222"), "GET", "/meus-convites", "", 200); body != "[]" {
		t.Fatalf("convites deviam ter sumido: %s", body)
	}
	esperar(t, aluno("111"), "POST", "/meu-pfc", `{"nome":"Athena 2"}`, 201)
}

func TestUmProjetoPorAlunoComConcorrencia(t *testing.T) {
	resetar(t)
	criarCursos(t, "Engenharia de Software")
	turma := id(t, esperar(t, admin, "POST", "/turmas", `{"nome":"Engenharia de Software","turno":"MANHA","periodo":"2027.1"}`, 201))
	prog := id(t, esperar(t, admin, "POST", "/programas", `{"cursoId":"`+turma+`"}`, 201))
	esperar(t, admin, "POST", "/admin/students", `{"turmaId":"`+turma+`","rgms":["555"]}`, 200)

	var projetos []string
	for i := 0; i < 5; i++ {
		projetos = append(projetos, id(t, esperar(t, admin, "POST", "/programas/"+prog+"/projetos", fmt.Sprintf(`{"nome":"G%d"}`, i), 201)))
	}

	// Cinco coordenadores põem o mesmo aluno em cinco grupos ao mesmo tempo:
	// só um pode conseguir.
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		sucesso int
	)
	for _, p := range projetos {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			if status, _ := chamar(t, admin, "PUT", "/projetos/"+p+"/integrantes", `{"rgm":"555"}`); status == 200 {
				mu.Lock()
				sucesso++
				mu.Unlock()
			}
		}(p)
	}
	wg.Wait()
	if sucesso != 1 {
		t.Fatalf("o aluno entrou em %d grupos, queria 1", sucesso)
	}
}

func TestCursos(t *testing.T) {
	resetar(t)
	eng := id(t, esperar(t, admin, "POST", "/cursos", `{"nome":"Engenharia de Software"}`, 201))
	si := id(t, esperar(t, admin, "POST", "/cursos", `{"nome":"Sistemas de Informação"}`, 201))
	// nome repetido, inclusive com outra caixa
	esperar(t, admin, "POST", "/cursos", `{"nome":"engenharia de software"}`, 409)

	// a turma só aceita curso cadastrado e grava o nome como ele foi cadastrado
	esperar(t, admin, "POST", "/turmas", `{"nome":"Medicina","turno":"NOITE","periodo":"2026.2"}`, 400)
	t1 := id(t, esperar(t, admin, "POST", "/turmas", `{"nome":"ENGENHARIA DE SOFTWARE","turno":"NOITE","periodo":"2026.2"}`, 201))
	t2 := id(t, esperar(t, admin, "POST", "/turmas", `{"nome":"Engenharia de Software","turno":"MANHA","periodo":"2026.2"}`, 201))
	if body := esperar(t, admin, "GET", "/turmas", "", 200); strings.Contains(body, "ENGENHARIA") {
		t.Fatalf("a turma devia guardar o nome do curso cadastrado: %s", body)
	}

	// curso com turmas não sai
	esperar(t, admin, "DELETE", "/cursos/"+eng, "", 409)

	// renomear leva as turmas junto, nome e trava
	esperar(t, admin, "PUT", "/cursos/"+eng, `{"nome":"Sistemas de Informação"}`, 409)
	esperar(t, admin, "PUT", "/cursos/"+eng, `{"nome":"Engenharia de Computação"}`, 200)
	body := esperar(t, admin, "GET", "/turmas", "", 200)
	if strings.Count(body, `"nome":"Engenharia de Computação"`) != 2 || strings.Contains(body, "de Software") {
		t.Fatalf("as turmas deviam ter o novo nome do curso: %s", body)
	}
	esperar(t, admin, "POST", "/turmas", `{"nome":"Engenharia de Software","turno":"NOITE","periodo":"2027.1"}`, 400)
	// a trava de turma acompanhou o nome: repetir a turma renomeada é conflito
	esperar(t, admin, "POST", "/turmas", `{"nome":"Engenharia de Computação","turno":"NOITE","periodo":"2026.2"}`, 409)
	// o nome antigo ficou livre para outro curso
	esperar(t, admin, "POST", "/cursos", `{"nome":"Engenharia de Software"}`, 201)

	// só a caixa muda: o nome continua o mesmo para a trava
	esperar(t, admin, "PUT", "/cursos/"+eng, `{"nome":"engenharia de computação"}`, 200)
	if body := esperar(t, admin, "GET", "/cursos", "", 200); !strings.Contains(body, `"nome":"engenharia de computação"`) {
		t.Fatalf("o curso devia ter o novo nome: %s", body)
	}
	esperar(t, admin, "POST", "/turmas", `{"nome":"Engenharia de Computação","turno":"NOITE","periodo":"2026.2"}`, 409)

	// sem turmas o curso sai, e o nome volta a ser inválido
	esperar(t, admin, "DELETE", "/turmas/"+t1, "", 200)
	esperar(t, admin, "DELETE", "/turmas/"+t2, "", 200)
	esperar(t, admin, "DELETE", "/cursos/"+eng, "", 200)
	esperar(t, admin, "DELETE", "/cursos/"+eng, "", 404)
	esperar(t, admin, "POST", "/turmas", `{"nome":"engenharia de computação","turno":"NOITE","periodo":"2026.2"}`, 400)
	esperar(t, admin, "DELETE", "/cursos/"+si, "", 200)
}

func TestAtividade(t *testing.T) {
	resetar(t)
	a := id(t, esperar(t, admin, "POST", "/atividades", `{"titulo":"Tema","prazo":"2099-11-01T23:59"}`, 201))
	body := esperar(t, admin, "PUT", "/atividades/"+a, `{"titulo":"Tema final","prazo":"2099-11-02T23:59"}`, 200)
	if !strings.Contains(body, `"titulo":"Tema final"`) || !strings.Contains(body, `"campos":[{`) {
		t.Fatalf("PUT devia devolver a atividade atualizada: %s", body)
	}
	esperar(t, admin, "PUT", "/atividades/inexistente", `{"titulo":"x","prazo":"2099-11-02T23:59"}`, 404)
	esperar(t, admin, "POST", "/atividades/inexistente/campos", `{"rotulo":"Link","tipo":"LINK"}`, 404)
	// Nasce com 1 campo; o limite é 20.
	for i := 1; i < 20; i++ {
		esperar(t, admin, "POST", "/atividades/"+a+"/campos", fmt.Sprintf(`{"rotulo":"Campo %d","tipo":"TEXTO"}`, i), 201)
	}
	esperar(t, admin, "POST", "/atividades/"+a+"/campos", `{"rotulo":"Demais","tipo":"TEXTO"}`, 409)
}

// TestTurmaAntiga covers a cohort written before courses existed: it carries
// only the course name and the lock keyed by it, until cmd/migrar runs.
func TestTurmaAntiga(t *testing.T) {
	resetar(t)
	ctx := context.Background()
	put := func(item map[string]types.AttributeValue) {
		if _, err := table.Client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(table.Name), Item: item}); err != nil {
			t.Fatal(err)
		}
	}
	s := func(v string) types.AttributeValue { return &types.AttributeValueMemberS{Value: v} }
	put(map[string]types.AttributeValue{"PK": s("CURSO#antiga"), "SK": s("PROFILE"), "GSI1PK": s("TURMAS"), "GSI1SK": s("CURSO#antiga"),
		"nome": s("Engenharia de Software"), "turno": s("NOITE"), "periodo": s("2026.2")})
	put(map[string]types.AttributeValue{"PK": s("TURMA_UNICA#Engenharia de Software#NOITE#2026.2"), "SK": s("TURMA_UNICA"), "cursoId": s("antiga")})
	curso := id(t, esperar(t, admin, "POST", "/cursos", `{"nome":"Engenharia de Software"}`, 201))

	if body := esperar(t, admin, "GET", "/turmas", "", 200); !strings.Contains(body, `"nome":"Engenharia de Software"`) {
		t.Fatalf("a turma antiga devia aparecer com o nome gravado: %s", body)
	}
	// editar liga a turma ao curso, troca a trava velha pela nova e conta a turma
	esperar(t, admin, "PUT", "/turmas/antiga", `{"nome":"Engenharia de Software","turno":"MANHA","periodo":"2026.2"}`, 200)
	velha, err := table.Client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(table.Name),
		Key: map[string]types.AttributeValue{"PK": s("TURMA_UNICA#Engenharia de Software#NOITE#2026.2"), "SK": s("TURMA_UNICA")}})
	if err != nil || velha.Item != nil {
		t.Fatalf("a trava antiga devia sair: %v %v", velha, err)
	}
	esperar(t, admin, "POST", "/turmas", `{"nome":"Engenharia de Software","turno":"MANHA","periodo":"2026.2"}`, 409)
	esperar(t, admin, "DELETE", "/cursos/"+curso, "", 409)
	esperar(t, admin, "DELETE", "/turmas/antiga", "", 200)
	esperar(t, admin, "DELETE", "/cursos/"+curso, "", 200)
}
