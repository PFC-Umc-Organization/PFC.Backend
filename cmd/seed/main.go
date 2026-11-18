package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/academic"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/deliveries"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/groups"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/identity"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/actor"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/platform/dynamo"
)

// ---------------------------------------------------------------------------
// Parâmetros
// ---------------------------------------------------------------------------

var (
	apply    = flag.Bool("aplicar", false, "grava no DynamoDB (sem a flag, só mostra o plano)")
	projects = flag.Int("projetos", 500, "quantidade de projetos")
	advisors = flag.Int("orientadores", 50, "quantidade de orientadores")
	workers  = flag.Int("paralelo", 8, "escritas simultâneas")
)

const (
	membersPerProject = 2
	pastActivities    = 4
	courseSuffix      = " (massa de teste)"
	studentPrefix     = "99"
)

var namespace = uuid.MustParse("6f1d2c3a-5b7e-4c9a-8d10-2a3b4c5d6e7f")

// ---------------------------------------------------------------------------
// Plano
// ---------------------------------------------------------------------------

// Modelo
type person struct {
	ID   string
	Name string
}

// Modelo
type plan struct {
	courses    []academic.Course
	cohorts    []academic.Cohort
	programs   []academic.Program
	activities [][]deliveries.Activity
	staff      []person
	students   []person
	enrolled   [][]string
	projects   []groups.Project
	advisorOf  []string
	profile    []int
	waivers    []waiver
	submits    []submit
}

// Modelo
type submit struct {
	sub deliveries.Submission
	val *deliveries.Validation
}

// Modelo
type waiver struct {
	projectID  string
	sk         string
	activityID string
	reason     string
	appliedAt  string
	appliedBy  string
	waivedFor  string
	waivedBy   string
	waivedAt   string
}

// Item
type penaltyRow struct {
	PK         string `dynamodbav:"PK"`
	SK         string `dynamodbav:"SK"`
	ActivityID string `dynamodbav:"atividadeId,omitempty"`
	Reason     string `dynamodbav:"motivo,omitempty"`
	AppliedAt  string `dynamodbav:"aplicadaEm,omitempty"`
	AppliedBy  string `dynamodbav:"aplicadaPor,omitempty"`
	WaivedFor  string `dynamodbav:"perdaoMotivo,omitempty"`
	WaivedBy   string `dynamodbav:"perdoadaPor,omitempty"`
	WaivedAt   string `dynamodbav:"perdoadaEm,omitempty"`
}

// Identificador
func id(parts ...any) string { return uuid.NewSHA1(namespace, []byte(fmt.Sprint(parts...))).String() }

var (
	firstNames = []string{"Ana", "Bruno", "Carla", "Diego", "Elisa", "Fábio", "Gabriela", "Henrique", "Isabela", "João",
		"Karina", "Lucas", "Mariana", "Nicolas", "Olívia", "Pedro", "Quésia", "Rafael", "Sofia", "Thiago", "Valéria", "Wesley"}
	lastNames = []string{"Almeida", "Barbosa", "Cardoso", "Dias", "Ferreira", "Gomes", "Henrique", "Lima", "Martins",
		"Nogueira", "Oliveira", "Pereira", "Ramos", "Santos", "Teixeira", "Vieira"}
	topics = []string{"Gestão de estoque", "Plataforma de tutoria", "Monitoramento de energia", "Aplicativo de saúde",
		"Agenda acadêmica", "Controle de frotas", "Marketplace local", "Análise de crédito", "Telemedicina",
		"Cidades inteligentes", "Rastreio de pedidos", "Educação financeira"}
	activityTitles = []string{"Proposta do projeto", "Plano de trabalho", "Revisão bibliográfica", "Protótipo inicial",
		"Relatório parcial", "Texto completo", "Ensaio da apresentação", "Entrega final"}
)

// Geração
func fullName(r *rand.Rand) string {
	return firstNames[r.Intn(len(firstNames))] + " " + lastNames[r.Intn(len(lastNames))] + " " + lastNames[r.Intn(len(lastNames))]
}

// Geração
func build(now time.Time) plan {
	r := rand.New(rand.NewSource(42))
	var p plan

	for _, name := range []string{"Engenharia de Software", "Sistemas de Informação"} {
		p.courses = append(p.courses, academic.Course{ID: id("course", name), Name: name + courseSuffix})
	}
	layout := []struct{ shift, term string }{{"MANHA", "2026.2"}, {"NOITE", "2026.2"}, {"NOITE", "2026.1"}}
	for _, c := range p.courses {
		for _, l := range layout {
			p.cohorts = append(p.cohorts, academic.Cohort{ID: id("cohort", c.ID, l.shift, l.term), CourseID: c.ID, Shift: l.shift, Term: l.term})
		}
	}
	for _, c := range p.cohorts {
		p.programs = append(p.programs, academic.Program{ID: id("program", c.ID), CohortID: c.ID})
	}
	p.activities = make([][]deliveries.Activity, len(p.cohorts))
	for ci, c := range p.cohorts {
		for k := 0; k < 6+ci%3; k++ {
			p.activities[ci] = append(p.activities[ci], activity(c, k, now))
		}
	}
	for i := 0; i < *advisors; i++ {
		p.staff = append(p.staff, person{ID: id("staff", i), Name: "Prof. " + fullName(r)})
	}
	p.enrolled = make([][]string, len(p.cohorts))
	for n := 0; n < *projects; n++ {
		ci := n % len(p.cohorts)
		pr := groups.Project{
			ID:          id("project", n),
			Name:        fmt.Sprintf("%s %d", topics[n%len(topics)], n+1),
			Description: "Projeto de massa de teste para carga e apresentação.",
			ProgramID:   p.programs[ci].ID,
		}
		for m := 0; m < membersPerProject; m++ {
			rgm := fmt.Sprintf("%s%09d", studentPrefix, len(p.students))
			p.students = append(p.students, person{ID: rgm, Name: fullName(r)})
			p.enrolled[ci] = append(p.enrolled[ci], rgm)
			pr.Members = append(pr.Members, rgm)
		}
		p.projects = append(p.projects, pr)
		p.advisorOf = append(p.advisorOf, p.staff[n%len(p.staff)].ID)
		p.profile = append(p.profile, n%20)
		p.progress(n, ci, now)
	}
	return p
}

// Atividade
func activity(c academic.Cohort, k int, now time.Time) deliveries.Activity {
	days := []int{-70, -52, -34, -16, 10, 28, 46, 64}[k]
	deadline := now.AddDate(0, 0, days)
	formats := []string{deliveries.FormatGeneral, deliveries.FormatRepository, deliveries.FormatVideo}
	return deliveries.Activity{
		ID:          id("activity", c.ID, k),
		Title:       activityTitles[k],
		Description: "Atividade do cronograma de massa de teste.",
		Deadline:    deadline.Format("2006-01-02") + "T23:59",
		PublishedAt: now.AddDate(0, 0, -90).UTC().Format(time.RFC3339),
		CohortID:    c.ID,
		Fields: []deliveries.Field{
			{ID: id("field", c.ID, k, 0), Label: "Resumo da entrega", Type: deliveries.FieldLongText, Required: true},
			{ID: id("field", c.ID, k, 1), Label: "Link do material", Type: deliveries.FieldLink, Required: true, Format: formats[k%3]},
			{ID: id("field", c.ID, k, 2), Label: "Arquivo da entrega", Type: deliveries.FieldFile, Format: "DOCUMENTO"},
		},
	}
}

// Resposta
func answers(a deliveries.Activity, n int) map[string]string {
	link := fmt.Sprintf("https://docs.exemplo.com.br/pfc/%d", n)
	switch a.Fields[1].Format {
	case deliveries.FormatRepository:
		link = fmt.Sprintf("https://github.com/pfc-seed/projeto-%d", n)
	case deliveries.FormatVideo:
		link = fmt.Sprintf("https://www.youtube.com/watch?v=seed%d", n)
	}
	return map[string]string{a.Fields[0].ID: "Resumo da entrega " + a.Title + ".", a.Fields[1].ID: link}
}

// Andamento
func (p *plan) progress(n, ci int, now time.Time) {
	project, advisor := p.projects[n], p.staff[n%len(p.staff)]
	acts := p.activities[ci]
	at := func(k int) time.Time {
		d, _ := time.Parse("2006-01-02T15:04", acts[k].Deadline)
		return d.AddDate(0, 0, -3)
	}
	put := func(k int, validation deliveries.ValidationStatus) {
		s := deliveries.Submission{
			ActivityID: acts[k].ID, ProjectID: project.ID, SubmittedAt: at(k).UTC().Format(time.RFC3339),
			SubmittedBy: p.students[len(p.students)-membersPerProject].Name, Answers: answers(acts[k], n),
		}
		sb := submit{sub: s}
		if validation != "" {
			v := deliveries.Validation{Status: validation, By: advisor.ID, ByName: advisor.Name, At: at(k).AddDate(0, 0, 1).UTC().Format(time.RFC3339)}
			if validation == deliveries.NotValidated {
				v.Comment = "Revise o texto e anexe as referências antes de enviar novamente."
			}
			sb.val = &v
		}
		p.submits = append(p.submits, sb)
	}
	forgive := func(k int) {
		p.waivers = append(p.waivers, waiver{
			projectID: project.ID, sk: "PENALIDADE#prazo-" + acts[k].ID, waivedFor: "Instabilidade comprovada no portal no dia do prazo.",
			waivedBy: advisor.Name, waivedAt: now.AddDate(0, 0, -5).UTC().Format(time.RFC3339),
		})
	}
	manual := func(waived bool) {
		w := waiver{
			projectID: project.ID, sk: "PENALIDADE#manual-" + id("manual", n), reason: "Ausência sem justificativa na reunião de orientação.",
			appliedAt: now.AddDate(0, 0, -20).UTC().Format(time.RFC3339), appliedBy: advisor.Name,
		}
		if waived {
			w.waivedFor, w.waivedBy, w.waivedAt = "Ausência justificada depois.", advisor.Name, now.AddDate(0, 0, -10).UTC().Format(time.RFC3339)
		}
		p.waivers = append(p.waivers, w)
	}

	switch pr := n % 20; {
	case pr <= 2:
	case pr == 3:
		for k := 0; k < 3; k++ {
			put(k, deliveries.Validated)
		}
	case pr == 4:
		for k := 0; k < 2; k++ {
			put(k, deliveries.Validated)
		}
	case pr == 5:
		for k := 0; k < 3; k++ {
			put(k, deliveries.Validated)
		}
		manual(false)
	case pr == 6 || pr == 7:
		for _, k := range []int{0, 1, 3} {
			put(k, deliveries.Validated)
		}
		forgive(2)
		if pr == 7 {
			manual(true)
		}
	case pr <= 10:
		for k := 0; k < pastActivities; k++ {
			status := deliveries.Validated
			if k == 3 || (pr == 9 && k == 2) {
				status = deliveries.NotValidated
			}
			put(k, status)
		}
	case pr <= 13:
		put(0, deliveries.Validated)
		for k := 1; k < pastActivities; k++ {
			put(k, "")
		}
	default:
		for k := 0; k < pastActivities; k++ {
			put(k, deliveries.Validated)
		}
		if pr <= 16 {
			put(pastActivities, "")
		}
	}
}

// ---------------------------------------------------------------------------
// Execução
// ---------------------------------------------------------------------------

// Execução
func main() {
	flag.Parse()
	name := os.Getenv("TABLE_NAME")
	if name == "" {
		log.Fatal("defina TABLE_NAME")
	}
	index := os.Getenv("GSI_NAME")
	if index == "" {
		index = "GSI1"
	}
	p := build(time.Now())
	fmt.Printf("plano: %d cursos, %d turmas, %d programas, %d atividades, %d orientadores, %d alunos, %d projetos, %d entregas, %d penalidades gravadas\n",
		len(p.courses), len(p.cohorts), len(p.programs), countActivities(p), len(p.staff), len(p.students), len(p.projects), len(p.submits), len(p.waivers))
	if !*apply {
		fmt.Println("simulação: nada foi gravado; use -aplicar para gravar na tabela", name)
		return
	}
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	table := &dynamo.Table{Client: dynamodb.NewFromConfig(cfg), Name: name, Index: index}
	if err := write(ctx, table, p); err != nil {
		log.Fatalf("seed: %v", err)
	}
	fmt.Println("concluído")
}

// Contagem
func countActivities(p plan) int {
	total := 0
	for _, a := range p.activities {
		total += len(a)
	}
	return total
}

// Gravação
func write(ctx context.Context, t *dynamo.Table, p plan) error {
	acad := academic.NewDynamoStore(t)
	grp := groups.NewDynamoStore(t)
	del := deliveries.NewDynamoStore(t)
	prof := identity.NewDynamoProfiles(t)

	if _, found, err := acad.CourseByName(ctx, p.courses[0].Name); err != nil {
		return fmt.Errorf("check seed: %w", err)
	} else if found {
		return fmt.Errorf("a massa de teste já existe nesta tabela")
	}

	steps := []struct {
		label string
		count int
		do    func(i int) error
	}{
		{"cursos", len(p.courses), func(i int) error { return acad.CreateCourse(ctx, p.courses[i]) }},
		{"turmas", len(p.cohorts), func(i int) error { return acad.CreateCohort(ctx, p.cohorts[i]) }},
		{"programas", len(p.programs), func(i int) error { return acad.CreateProgram(ctx, p.programs[i]) }},
		{"atividades", len(p.cohorts), func(i int) error {
			for _, a := range p.activities[i] {
				if err := del.CreateActivity(ctx, a); err != nil {
					return err
				}
			}
			return nil
		}},
		{"orientadores", len(p.staff), func(i int) error { return prof.SaveStaff(ctx, p.staff[i].ID, p.staff[i].Name, actor.RoleAdvisor) }},
		{"alunos", len(p.students), func(i int) error { return prof.SaveStudent(ctx, p.students[i].ID, p.students[i].Name) }},
		{"matrículas", len(p.cohorts), func(i int) error {
			for rgms := p.enrolled[i]; len(rgms) > 0; {
				n := min(25, len(rgms))
				if err := acad.PutEnrollments(ctx, rgms[:n], p.cohorts[i].ID); err != nil {
					return err
				}
				rgms = rgms[n:]
			}
			return nil
		}},
		{"projetos", len(p.projects), func(i int) error {
			if err := grp.CreateProject(ctx, p.projects[i]); err != nil {
				return err
			}
			return grp.SetAdvisor(ctx, p.projects[i].ID, p.advisorOf[i])
		}},
		{"entregas", len(p.submits), func(i int) error {
			s := p.submits[i]
			if err := del.PutSubmission(ctx, s.sub, nil); err != nil {
				return err
			}
			if s.val == nil {
				return nil
			}
			_, err := del.SetValidation(ctx, s.sub.ProjectID, s.sub.ActivityID, *s.val)
			return err
		}},
		{"penalidades", len(p.waivers), func(i int) error {
			w := p.waivers[i]
			return dynamo.PutStruct(ctx, t, penaltyRow{
				PK: "PROJECT#" + w.projectID, SK: w.sk, ActivityID: w.activityID, Reason: w.reason,
				AppliedAt: w.appliedAt, AppliedBy: w.appliedBy, WaivedFor: w.waivedFor, WaivedBy: w.waivedBy, WaivedAt: w.waivedAt,
			}, dynamo.Write{})
		}},
	}
	for _, s := range steps {
		if err := each(s.count, s.do); err != nil {
			return fmt.Errorf("%s: %w", s.label, err)
		}
		fmt.Printf("%-13s %d\n", s.label, s.count)
	}
	return nil
}

// Paralelismo
func each(n int, do func(i int) error) error {
	var (
		wg    sync.WaitGroup
		next  atomic.Int64
		first atomic.Value
	)
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for first.Load() == nil {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				if err := do(i); err != nil {
					first.CompareAndSwap(nil, fmt.Errorf("item %d: %w", i, err))
				}
			}
		}()
	}
	wg.Wait()
	if err, ok := first.Load().(error); ok {
		return err
	}
	return nil
}
