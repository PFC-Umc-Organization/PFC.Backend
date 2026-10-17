# PFC.Backend

Backend em Go do PFC Manager — um único Lambda com router interno atrás de
uma API Gateway REST API.

## Estrutura

```
main.go                    # entrypoint do Lambda: app.Load + app.New
cmd/migrar/                # migração idempotente da tabela pra modelagem atual
internal/
├── app/                   # composition root: clientes AWS, config e rotas
├── platform/              # infraestrutura compartilhada pelos módulos
│   ├── config/            # variáveis de ambiente obrigatórias
│   ├── httpapi/           # router, Request/Response, erro → status, CORS, recover
│   ├── apperr/            # erros de aplicação (tipo + mensagem pro usuário)
│   ├── dynamo/            # helpers do DynamoDB (query paginada, lotes, transações)
│   ├── actor/             # quem chama (claims do Cognito), perfis e RGM
│   └── audit/             # eventos de auditoria no CloudWatch
├── common/ auditoria/     # ponte temporária pros pacotes antigos (sai na fase 7)
├── integracao/            # testes ponta a ponta contra o DynamoDB Local
├── auth/                  # domínio de autenticação (ponte com o Cognito)
│   ├── models.go          # espelha os tipos TS do frontend
│   ├── cognito.go          # chamadas ao Cognito (InitiateAuth, SignUp)
│   └── handler.go          # handlers HTTP: POST /auth/login, /auth/registrar
├── academic/              # módulo: turmas, programa da turma e allowlist de RGMs
│   ├── domain.go          # tipos, validação, mensagens
│   ├── ports.go           # interfaces que o serviço usa (Store, Projects)
│   ├── service.go         # regras de negócio + auditoria
│   ├── store.go           # DynamoDB: chaves, travas, transações
│   └── http.go            # rotas /turmas, /programas, /admin/students
├── groups/                # módulo: projetos, integrantes, orientador e convites
│   └── (mesmos arquivos)  # store.go: PK PROJECT#id, GSI1 por programa, vínculo MEMBRO#rgm
├── deliveries/            # módulo: atividades, campos de entrega (formulário) e entregas dos grupos
└── referencia/             # referências bibliográficas (APIs externas OpenAlex + Crossref)
    ├── openalex.go          # busca de artigos por tema
    ├── crossref.go          # metadados completos de uma obra pelo DOI
    ├── abnt.go              # formatação ABNT NBR 6023 (texto e HTML)
    ├── dynamo.go            # PK PROJECT#id, SK REF#<hash do DOI>
    └── handler.go           # handlers HTTP: /referencias/*, /projetos/:id/referencias
```

Cada novo domínio (atividade, material, usuário, etc.) ganha seu próprio
pacote em `internal/`, registrado em `internal/app/routes.go` — nenhuma rota existente
precisa mudar.

## Variáveis de ambiente

| Variável            | Descrição                                    |
| ------------------- | --------------------------------------------- |
| `COGNITO_CLIENT_ID` | App Client ID do Cognito (SPA, sem secret)    |
| `COGNITO_USER_POOL_ID` | Id do User Pool — usado pelo `ListUsers` de `GET /usuarios` (o Terraform deriva do ARN) |
| `TABLE_NAME`        | Nome da tabela DynamoDB                       |
| `GSI_NAME`          | Nome do índice secundário GSI1 (ex: `GSI1`), com projeção ALL |
| `REFERENCIAS_CONTATO_EMAIL` | Opcional. E-mail de contato enviado ao Crossref/OpenAlex (dá prioridade no "polite pool") |
| `OPENALEX_API_KEY`  | Opcional. Chave da OpenAlex pra limites maiores — funciona sem |

As quatro primeiras são obrigatórias: sem elas a Lambda falha no init, com
a lista do que falta no log (`platform/config`).

## Modelagem no DynamoDB

Uma tabela (`PK`, `SK`) com um índice, o GSI1 (`GSI1PK`, `GSI1SK`). Toda
consulta do dia a dia é `GetItem`, `Query` ou `BatchGetItem` — não há Scan
em rota nenhuma.

| Item | PK | SK | GSI1PK | GSI1SK |
| ---- | -- | -- | ------ | ------ |
| Turma | `CURSO#<id>` | `PROFILE` | `TURMAS` | `CURSO#<id>` |
| Trava de turma | `TURMA_UNICA#<curso>#<turno>#<período>` | `TURMA_UNICA` | — | — |
| Programa da turma | `CURSO#<id>` | `PROGRAMA` | — | — |
| Programa | `PROGRAM#<id>` | `PROFILE` | `PROGRAMAS` | `PROGRAM#<id>` |
| Matrícula | `STUDENT#<rgm>` | `PROFILE` | `MATRICULAS` | `STUDENT#<rgm>` |
| Projeto | `PROJECT#<id>` | `PROFILE` | `PROGRAM#<pid>` | `PROJECT#<id>` |
| Projeto do aluno | `MEMBRO#<rgm>` | `PROJETO` | — | — |
| Convite | `PROJECT#<id>` | `CONVITE#<rgm>` | `CONVITE#<rgm>` | `PROJECT#<id>` |
| Entrega | `PROJECT#<id>` | `ENTREGA#<aid>` | `ENTREGAS#<aid>` | `PROJECT#<id>` |
| Referência | `PROJECT#<id>` | `REF#<hash do DOI>` | — | — |
| Atividade | `ACTIVITY#<id>` | `PROFILE` | `ACTIVITIES` | `ACTIVITY#<id>` |

As regras de unicidade são garantidas pelo próprio DynamoDB, numa
`TransactWriteItems` com `attribute_not_exists`, e não por "ler e depois
gravar":

- **uma turma por curso+turno+período** — a trava é gravada junto com a turma;
- **um programa por turma** — o item "programa da turma" é gravado junto com o
  programa (e responde "qual o programa desta turma?" com um `GetItem`);
- **um projeto por aluno** — o item "projeto do aluno" é gravado junto com a
  entrada do RGM em `integrantes` (criação pelo aluno, aceite de convite e
  inclusão pela coordenação) e apagado junto com a saída.

Tabela que já existia antes dessa modelagem precisa da migração (uma vez):
ver [Migração](#migração).

## Rotas implementadas

| Método | Rota                                    | Descrição                                                        | Autorização |
| ------ | ---------------------------------------- | ------------------------------------------------------------------ | ----------- |
| POST   | `/auth/login`                            | Autentica via Cognito, devolve `{ usuario, token }`                | pública |
| POST   | `/auth/registrar`                        | Cadastro self-service de aluno. `perfil` do corpo é ignorado — sempre ALUNO | pública |
| GET    | `/usuarios`                              | Contas do Cognito (`ListUsers`, paginado): nome, perfil, status, `confirmado` e `rgm` (aluno) | qualquer autenticado — e-mail só pra ORIENTADOR/ADMIN |
| POST   | `/admin/students`                        | Grava RGMs na allowlist (`STUDENT#<rgm>` ACTIVE). RGM com algo além de dígitos volta em `falhas` | ADMIN |
| DELETE | `/admin/students`                        | Remove RGMs da allowlist                                            | ADMIN |
| POST   | `/programas`                             | Cria Programa (`{ cursoId }`)                                       | ADMIN |
| GET    | `/programas`                             | Lista programas                                                     | qualquer autenticado |
| POST   | `/meu-pfc`                               | Aluno pré-autorizado cria o projeto do grupo (`{ nome, descricao, integrantes: RGMs }`) no programa da própria turma; só ele entra como integrante, os colegas recebem **convite** (ver `/meus-convites`). 403 sem turma, 409 sem programa na turma ou integrante já em projeto, 400 colega fora da turma | ALUNO |
| POST   | `/programas/:programaId/projetos`        | Cria projeto vinculado ao programa (`{ nome, descricao }`; `integrantes` do corpo é ignorado — entram por `PUT /projetos/:id/integrantes`) | ADMIN |
| GET    | `/programas/:programaId/projetos`        | Lista projetos do programa                                          | qualquer autenticado |
| PUT    | `/projetos/:projetoId/orientador`        | Associa orientador (`{ orientadorId }`) ao projeto                  | ADMIN |
| GET    | `/meus-convites`                         | Convites pendentes recebidos pelo aluno logado                      | ALUNO |
| POST   | `/meus-convites/:projetoId/aceitar`      | Aceita: entra no grupo (revalida turma, se está livre e o limite de 10) e apaga os outros convites dele. 404 sem convite ou grupo apagado, 409 já em projeto/fora da turma/grupo cheio | ALUNO |
| POST   | `/meus-convites/:projetoId/recusar`      | Recusa (apaga o convite)                                            | ALUNO |
| GET    | `/projetos/:projetoId/convites`          | Convites pendentes do grupo                                         | integrantes, ADMIN, ORIENTADOR |
| POST   | `/projetos/:projetoId/convites`          | Convida mais um colega (`{ rgm }`), com as regras do `/meu-pfc`; convite pendente conta no limite de 10 | integrantes do projeto |
| DELETE | `/projetos/:projetoId/convites/:rgm`     | Cancela um convite                                                  | integrantes do projeto |
| GET    | `/atividades`                            | Lista atividades (com `campos`), ordenadas por prazo               | qualquer autenticado |
| POST   | `/atividades`                            | Cria atividade (`{ titulo, descricao, prazo, campos? }`); sem `campos` nasce com 1 campo de arquivo obrigatório | ADMIN, ORIENTADOR |
| PUT    | `/atividades/:atividadeId`               | Edita título/descrição/prazo                                       | ADMIN, ORIENTADOR |
| DELETE | `/atividades/:atividadeId`               | Remove a atividade e as entregas dela                               | ADMIN, ORIENTADOR |
| POST   | `/atividades/:atividadeId/campos`        | Adiciona campo (`{ rotulo, tipo, obrigatorio }`; tipos ARQUIVO/TEXTO/TEXTO_LONGO/LINK; máx. 20) | ADMIN, ORIENTADOR |
| DELETE | `/atividades/:atividadeId/campos/:campoId` | Remove campo; 409 se for o último                                | ADMIN, ORIENTADOR |
| GET    | `/atividades/:atividadeId/entregas`      | Entregas de todos os projetos naquela atividade                     | ADMIN, ORIENTADOR |
| GET    | `/projetos/:projetoId/entregas`          | Entregas do grupo                                                   | integrantes, ADMIN, ORIENTADOR |
| PUT    | `/projetos/:projetoId/entregas/:atividadeId` | Entrega/reenvia (`{ respostas: { <campoId>: valor } }`), validada contra os campos | integrantes do projeto |
| DELETE | `/projetos/:projetoId/entregas/:atividadeId` | Remove a entrega do grupo (devolve pra refazer)                  | ADMIN, ORIENTADOR |
| GET    | `/referencias/busca?q=&pagina=`          | Busca artigos por tema na **OpenAlex** (10 por página, até 50 páginas) | qualquer autenticado |
| GET    | `/referencias/doi?doi=`                  | Consulta o DOI no **Crossref** e devolve a referência em ABNT (`abnt`, `abntHtml`) | qualquer autenticado |
| GET    | `/projetos/:projetoId/referencias`       | Lista de referências do projeto, em ordem alfabética                | integrantes, ORIENTADOR, ADMIN |
| POST   | `/projetos/:projetoId/referencias`       | Adiciona pelo DOI (`{ doi }`); 409 se já estiver na lista           | integrantes do projeto |
| DELETE | `/projetos/:projetoId/referencias/:referenciaId` | Remove da lista                                             | integrantes do projeto |

## Decisões importantes

- **`perfil` nunca vem do cliente em `/auth/registrar`.** Endpoint público;
  se o valor enviado no corpo fosse usado pra decidir o perfil, qualquer
  requisição poderia se autodeclarar ADMIN. Cadastro público sempre
  cria ALUNO; contas de professor/coordenador só existem via
  `AdminCreateUser`, com `custom:perfil` setado explicitamente por quem
  cria a conta.
- **Perfil vem do atributo customizado `custom:perfil`, não de grupo
  Cognito.** Grupo é mecanismo de infraestrutura (mapeamento de IAM Role,
  usado no harness pra dar credenciais AWS); perfil é dado de negócio (quem
  essa pessoa é). Ver `internal/platform/httpapi/request.go`.
- **Token não é validado no login.** O IdToken usado pra montar a resposta
  acabou de ser emitido pelo Cognito na mesma chamada — validar assinatura
  importa em rotas protegidas (via authorizer do API Gateway), não aqui.
- **Programa referencia a turma por id, não duplica dados.** A transação
  que cria o programa confere que a turma existe.
- **API Gateway usa dois proxies genéricos** (`/auth/{proxy+}` sem
  authorizer, `/{proxy+}` com Cognito Authorizer) — nenhuma rota nova
  exige mudança de Terraform, só registrar em `internal/app/routes.go`. O router
  interno (`internal/platform/httpapi`) é quem resolve path parameters (`:programaId` etc.).
- **Convite de integrante fica no sistema, sem e-mail (por enquanto).**
  Não temos SES; o convite é salvo no DynamoDB (`PK PROJECT#<id>`,
  `SK CONVITE#<rgm>`, GSI1 `CONVITE#<rgm>` pra listar os recebidos) e o
  aluno responde pela tela. O envio de e-mail é um gancho único,
  `notify` do `groups.Service`, que hoje só loga.
  Ninguém vira integrante sem aceitar. O aceite grava com condição atômica
  (sem duplicar o RGM nem passar de 10) e com o vínculo "projeto do aluno"
  na mesma transação: dois aceites simultâneos em grupos diferentes não põem
  o aluno em dois grupos.
- **Referências: o front nunca chama OpenAlex/Crossref direto.** A
  integração fica no backend, que também formata em ABNT. O `POST` recebe
  só o DOI e busca os metadados de novo no Crossref — o aluno não consegue
  gravar referência com dados inventados. A lista é do grupo (igual à
  entrega): só integrante altera; professor/coordenador acompanham.
- **Integrante é identificado pelo RGM do e-mail.** O e-mail de aluno é
  `<rgm>@alunos.umc.br` (garantido pelo Pre Sign-up), então
  `actor.Actor.RGM()` usa a parte local do claim `email`.
- **`cursoIds` do Usuario vem da matrícula** — a turma em que o RGM foi
  pré-autorizado (`academic.Service.CohortOf`); vazio pra quem não é aluno.
- **Um cliente AWS por processo.** `app.Load` cria os clientes do DynamoDB e
  do Cognito uma vez, na fase de init da Lambda, e `app.New` os entrega a
  todos os pacotes (um pool de conexões só).

## Build

```bash
GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -trimpath -ldflags="-s -w" -o bootstrap main.go
zip function.zip bootstrap
```

Runtime esperado no Lambda: `provided.al2023`, arquitetura `arm64`.
`-ldflags="-s -w"` tira a tabela de símbolos e o DWARF: o binário cai de
13,1 MB pra 9,0 MB e o zip de 6,7 MB pra 3,3 MB — menos pra baixar e
descompactar a cada cold start.

## Testes

```bash
go mod tidy        # o repo não versiona go.sum
go test ./...

# contra as APIs reais do Crossref/OpenAlex (precisa de internet)
REFERENCIAS_INTEGRACAO=1 go test ./internal/referencia -run Integracao -v
```

# ponta a ponta: precisa do DynamoDB Local rodando (porta 8000 por padrão, ver
# https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/DynamoDBLocal.html)
scripts/test-integration.sh
DYNAMODB_LOCAL=http://localhost:8765 scripts/test-integration.sh   # outra porta
```

O pacote `internal/integracao` usa o DynamoDB Local de verdade e servidores
falsos para o Cognito (`cognito_fake_test.go`) e para Crossref/OpenAlex. Sem o
script, ele é pulado.

- **Contrato** (`contrato_test.go`): percorre as 47 rotas, com casos de sucesso
  e de erro, e compara status e corpo de cada resposta com
  `testdata/contrato.golden` (ids e datas normalizados). Qualquer mudança na API
  quebra o teste; se a mudança for intencional, regrave e revise o diff:
  `scripts/test-integration.sh -run Contrato -update`.
- **Regras com banco** (`integracao_test.go`): unicidades, um projeto por aluno
  com cinco inclusões simultâneas, cascata ao apagar.

## Migração

Tabela criada antes da modelagem atual (sem GSI nas turmas, programas e
matrículas, sem travas nem vínculos) precisa passar uma vez pelo
`cmd/migrar`. É idempotente e, sem `-aplicar`, só simula:

```bash
TABLE_NAME=<tabela> go run ./cmd/migrar            # mostra o que faria
TABLE_NAME=<tabela> go run ./cmd/migrar -aplicar
```

Conflitos (duas turmas iguais, dois programas na mesma turma, um aluno em
dois projetos) aparecem como `CONFLITO:` e precisam de correção manual —
a migração não escolhe qual lado fica.
