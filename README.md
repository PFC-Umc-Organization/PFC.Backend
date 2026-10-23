# PFC.Backend

Backend em Go do PFC Manager — um único Lambda com router interno atrás de
uma API Gateway REST API.

## Estrutura

```
main.go                    # entrypoint do Lambda: app.Load + app.New
cmd/migrar/                # migração idempotente da tabela pra modelagem atual
internal/
├── app/                   # composition root: clientes AWS, config e ligação dos módulos
├── platform/              # infraestrutura compartilhada pelos módulos
│   ├── config/            # variáveis de ambiente
│   ├── httpapi/           # router, Request/Response, perfil por rota, erro → status, CORS, recover
│   ├── apperr/            # erros de aplicação (tipo + mensagem pro usuário)
│   ├── dynamo/            # helpers do DynamoDB (query paginada, lotes, transações)
│   ├── actor/             # quem chama (claims do Cognito), perfis e RGM
│   ├── audit/             # eventos de auditoria no CloudWatch
│   ├── parallel/          # consultas em paralelo dentro de um request (no máximo 8)
│   └── storage/           # arquivos no S3: POST/GET assinados, tipos aceitos, 50 MB
├── identity/              # módulo: login, cadastro, confirmação e contas (Cognito)
├── academic/              # módulo: cursos, turmas, programa da turma e allowlist de RGMs
├── groups/                # módulo: projetos, integrantes, orientador e convites
├── deliveries/            # módulo: atividades, campos de entrega e entregas dos grupos (com arquivo)
├── materials/             # módulo: materiais de apoio (link ou arquivo), por turma ou gerais
├── references/            # módulo: referências bibliográficas (OpenAlex, Crossref, ABNT)
└── integracao/            # testes ponta a ponta contra o DynamoDB Local
```

Todo módulo tem o mesmo formato:

| Arquivo      | Conteúdo |
| ------------ | -------- |
| `domain.go`  | tipos, validação e mensagens (em português, como a API responde) |
| `ports.go`   | interfaces que o serviço usa: o store e os outros módulos |
| `service.go` | regras de negócio e auditoria; não conhece HTTP nem DynamoDB |
| `store.go`   | DynamoDB: chaves, travas e transações (no `identity`, `cognito.go`; no `references`, também `catalog.go`) |
| `http.go`    | rotas com o perfil exigido; só decodifica o corpo e chama o serviço |
| `*_test.go`  | testes do serviço com fakes em memória |

Um módulo usa outro só pelas interfaces do `ports.go`, montadas em
`internal/app`. Rota nova entra no `http.go` do módulo — o API Gateway
encaminha tudo pra Lambda.


## Variáveis de ambiente

| Variável            | Descrição                                    |
| ------------------- | --------------------------------------------- |
| `COGNITO_CLIENT_ID` | App Client ID do Cognito (SPA, sem secret)    |
| `COGNITO_USER_POOL_ID` | Id do User Pool — usado pelo `ListUsers` de `GET /usuarios` (o Terraform deriva do ARN) |
| `TABLE_NAME`        | Nome da tabela DynamoDB                       |
| `GSI_NAME`          | Nome do índice secundário GSI1 (ex: `GSI1`), com projeção ALL |
| `REFERENCIAS_CONTATO_EMAIL` | Opcional. E-mail de contato enviado ao Crossref/OpenAlex (dá prioridade no "polite pool") |
| `OPENALEX_API_KEY`  | Opcional. Chave da OpenAlex pra limites maiores — funciona sem |
| `UPLOADS_BUCKET`    | Opcional. Bucket privado dos arquivos (entregas e materiais). Sem ele, campo de arquivo guarda só o nome e material é só link |

As quatro primeiras são obrigatórias: sem elas a Lambda falha no init, com
a lista do que falta no log (`platform/config`).

## Modelagem no DynamoDB

Uma tabela (`PK`, `SK`) com um índice, o GSI1 (`GSI1PK`, `GSI1SK`). Toda
consulta do dia a dia é `GetItem`, `Query` ou `BatchGetItem` — não há Scan
em rota nenhuma.

| Item | PK | SK | GSI1PK | GSI1SK |
| ---- | -- | -- | ------ | ------ |
| Curso (`nome`, `turmas` = quantas turmas tem) | `COURSE#<id>` | `PROFILE` | `COURSES` | `COURSE#<id>` |
| Trava do nome do curso | `CURSO_NOME#<nome em minúsculas>` | `CURSO_NOME` | — | — |
| Turma (`graduacaoId` = id do curso, `turno`, `periodo`) | `CURSO#<id>` | `PROFILE` | `TURMAS` | `CURSO#<id>` |
| Trava de turma | `TURMA_UNICA#<id do curso>#<turno>#<período>` | `TURMA_UNICA` | — | — |
| Programa da turma | `CURSO#<id>` | `PROGRAMA` | — | — |
| Programa | `PROGRAM#<id>` | `PROFILE` | `PROGRAMAS` | `PROGRAM#<id>` |
| Matrícula | `STUDENT#<rgm>` | `PROFILE` | `MATRICULAS` | `STUDENT#<rgm>` |
| Projeto | `PROJECT#<id>` | `PROFILE` | `PROGRAM#<pid>` | `PROJECT#<id>` |
| Projeto do aluno | `MEMBRO#<rgm>` | `PROJETO` | — | — |
| Convite | `PROJECT#<id>` | `CONVITE#<rgm>` | `CONVITE#<rgm>` | `PROJECT#<id>` |
| Entrega | `PROJECT#<id>` | `ENTREGA#<aid>` | `ENTREGAS#<aid>` | `PROJECT#<id>` |
| Referência | `PROJECT#<id>` | `REF#<hash do DOI>` | — | — |
| Atividade | `ACTIVITY#<id>` | `PROFILE` | `ACTIVITIES` | `ACTIVITY#<id>` |
| Material | `MATERIAL#<id>` | `PROFILE` | `MATERIAIS` | `MATERIAL#<id>` |

Arquivos ficam no bucket `UPLOADS_BUCKET`, nunca no DynamoDB:
`entregas/<projeto>/<atividade>/<campo>/<id>/<nome>` (a resposta do campo é a
chave) e `materiais/<id>/<nome>`.

As regras de unicidade são garantidas pelo próprio DynamoDB, numa
`TransactWriteItems` com `attribute_not_exists`, e não por "ler e depois
gravar":

- **um curso por nome** (sem diferenciar maiúsculas nem espaços repetidos) — a
  trava do nome é gravada junto com o curso;
- **curso com turma não sai** — a turma aponta para o curso pelo id e o curso
  conta as turmas (`turmas`) nas mesmas transações que criam, mudam e apagam
  turma; apagar exige `turmas = 0`. Renomear o curso mexe só nele: as turmas
  mostram o nome novo na próxima listagem;
- **uma turma por curso+turno+período** — a trava é gravada junto com a turma.
  O período tem uma grafia só, `AAAA.1` ou `AAAA.2` (`2026/2` e `2026-2` viram
  `2026.2`), senão a mesma turma passaria com outra escrita;

`CURSO#` é a **turma** e `COURSE#` o **curso**: o prefixo das turmas é anterior
ao cadastro de cursos e não pode mudar sem migrar a tabela inteira.
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
| POST   | `/auth/confirmar`                        | Confirma a conta com o código do e-mail (`{ email, codigo }`)       | pública |
| POST   | `/auth/reenviar-codigo`                  | Manda outro código de confirmação (`{ email }`)                     | pública |
| POST   | `/auth/nova-senha`                       | Primeiro acesso de conta criada pelo admin: o login devolve `{ desafio: "NOVA_SENHA", sessao }` e esta rota troca a senha temporária (`{ email, sessao, senha }`), devolvendo `{ usuario, token }` | pública |
| POST   | `/auth/esqueci-senha`                    | Manda o código de redefinição (`{ email }`); responde igual exista ou não a conta | pública |
| POST   | `/auth/redefinir-senha`                  | Define a senha nova com o código (`{ email, codigo, senha }`)       | pública |
| GET    | `/me`                                    | O usuário logado, com a turma lida agora (o front relê ao abrir)    | qualquer autenticado |
| GET    | `/usuarios`                              | Contas do Cognito (`ListUsers`, paginado): nome, e-mail, perfil, status, `confirmado`, `rgm` (aluno) e a turma (um `BatchGetItem` para todos) | ADMIN, ORIENTADOR |
| GET    | `/cursos`                                | Lista os cursos cadastrados                                         | qualquer autenticado |
| POST   | `/cursos`                                | Cadastra curso (`{ nome }`); 409 se o nome já existe                | ADMIN |
| PUT    | `/cursos/:cursoId`                       | Renomeia; as turmas mostram o nome novo                             | ADMIN |
| DELETE | `/cursos/:cursoId`                       | Remove; 409 se o curso tem turmas                                   | ADMIN |
| POST   | `/admin/students`                        | Grava RGMs na allowlist (`STUDENT#<rgm>` ACTIVE). RGM com algo além de dígitos volta em `falhas` | ADMIN |
| DELETE | `/admin/students`                        | Remove RGMs da allowlist                                            | ADMIN |
| POST   | `/programas`                             | Cria Programa (`{ cursoId }`)                                       | ADMIN |
| GET    | `/programas`                             | Lista programas                                                     | qualquer autenticado |
| GET    | `/meu-pfc`                               | O grupo do aluno logado no formato de `GET /projetos/:id` (`grupo`), ou `grupo: null` com a turma em que está pré-autorizado (`turma`, lida agora) e o programa dela (`programaId`, ausente se a turma não tem PFC) | ALUNO |
| POST   | `/meu-pfc`                               | Aluno pré-autorizado cria o projeto do grupo (`{ nome, descricao, integrantes: RGMs }`) no programa da própria turma; só ele entra como integrante, os colegas recebem **convite** (ver `/meus-convites`). 403 sem turma, 409 sem programa na turma ou integrante já em projeto, 400 colega fora da turma | ALUNO |
| POST   | `/programas/:programaId/projetos`        | Cria projeto vinculado ao programa (`{ nome, descricao }`; `integrantes` do corpo é ignorado — entram por `PUT /projetos/:id/integrantes`) | ADMIN |
| GET    | `/programas/:programaId/projetos`        | Lista projetos do programa; o orientador recebe só os que orienta   | ADMIN, ORIENTADOR |
| GET    | `/projetos/:projetoId`                   | Projeto com os nomes: `{ projeto, integrantes: [{ rgm, nome }], orientador: { id, nome }, turma }` (`nome` null = RGM sem conta). Nomes e turma que falharem ficam vazios em vez de derrubar a tela | integrantes, orientador do projeto, ADMIN |
| PUT    | `/projetos/:projetoId/orientador`        | Associa orientador (`{ orientadorId }`) ao projeto                  | ADMIN |
| GET    | `/meus-convites`                         | Convites pendentes recebidos pelo aluno logado                      | ALUNO |
| POST   | `/meus-convites/:projetoId/aceitar`      | Aceita: entra no grupo (revalida turma, se está livre e o limite de 10) e apaga os outros convites dele. 404 sem convite ou grupo apagado, 409 já em projeto/fora da turma/grupo cheio | ALUNO |
| POST   | `/meus-convites/:projetoId/recusar`      | Recusa (apaga o convite)                                            | ALUNO |
| GET    | `/projetos/:projetoId/convites`          | Convites pendentes do grupo                                         | integrantes, orientador do projeto, ADMIN |
| POST   | `/projetos/:projetoId/convites`          | Convida mais um colega (`{ rgm }`), com as regras do `/meu-pfc`; convite pendente conta no limite de 10 | integrantes do projeto |
| DELETE | `/projetos/:projetoId/convites/:rgm`     | Cancela um convite                                                  | integrantes do projeto |
| GET    | `/atividades`                            | Lista atividades (com `campos`), ordenadas por prazo               | qualquer autenticado |
| POST   | `/atividades`                            | Cria atividade (`{ titulo, descricao, prazo, campos? }`); sem `campos` nasce com 1 campo de arquivo obrigatório | ADMIN, ORIENTADOR |
| PUT    | `/atividades/:atividadeId`               | Edita título/descrição/prazo                                       | ADMIN, ORIENTADOR |
| DELETE | `/atividades/:atividadeId`               | Remove a atividade e as entregas dela                               | ADMIN, ORIENTADOR |
| POST   | `/atividades/:atividadeId/campos`        | Adiciona campo (`{ rotulo, tipo, obrigatorio }`; tipos ARQUIVO/TEXTO/TEXTO_LONGO/LINK; máx. 20) | ADMIN, ORIENTADOR |
| DELETE | `/atividades/:atividadeId/campos/:campoId` | Remove campo; 409 se for o último                                | ADMIN, ORIENTADOR |
| GET    | `/atividades/:atividadeId/entregas`      | Entregas de todos os projetos naquela atividade                     | ADMIN |
| GET    | `/programas/:programaId/entregas`        | Entregas (com respostas) de todos os projetos do programa, numa chamada — a matriz do painel; o orientador recebe só as dos projetos que orienta | ADMIN, ORIENTADOR |
| GET    | `/projetos/:projetoId/entregas`          | Entregas do grupo                                                   | integrantes, orientador do projeto, ADMIN |
| PUT    | `/projetos/:projetoId/entregas/:atividadeId` | Entrega/edita (`{ respostas: { <campoId>: valor } }`), validada contra os campos; depois do prazo (horário de Brasília) só edita quem entregou (409) | integrantes do projeto |
| DELETE | `/projetos/:projetoId/entregas/:atividadeId` | Remove a entrega do grupo (devolve pra refazer) e apaga os arquivos dela | ADMIN, orientador do projeto |
| POST   | `/projetos/:projetoId/entregas/:atividadeId/arquivos` | Pede o envio de um arquivo para um campo ARQUIVO (`{ campoId, nome, tamanho }`) → `{ url, campos, chave, expiraEm, tamanhoMaximo }`: o navegador manda `campos` + o arquivo (campo `file`) em multipart para `url`, e a `chave` vira a resposta do campo no `PUT` da entrega. 400 tipo não aceito ou acima de 50 MB; 503 sem `UPLOADS_BUCKET` | integrantes do projeto |
| GET    | `/projetos/:projetoId/entregas/:atividadeId/arquivos/:campoId` | `{ url, nome }`: link de download de 5 minutos. 404 se a resposta é só um nome (entrega de antes do upload) | integrantes, orientador do projeto, ADMIN |
| GET    | `/referencias/busca?q=&pagina=`          | Busca artigos por tema na **OpenAlex** (10 por página, até 50 páginas) | qualquer autenticado |
| GET    | `/referencias/doi?doi=`                  | Consulta o DOI no **Crossref** e devolve a referência em ABNT (`abnt`, `abntHtml`) | qualquer autenticado |
| GET    | `/projetos/:projetoId/referencias`       | Lista de referências do projeto, em ordem alfabética                | integrantes, orientador do projeto, ADMIN |
| POST   | `/projetos/:projetoId/referencias`       | Adiciona pelo DOI (`{ doi }`); 409 se já estiver na lista           | integrantes do projeto |
| DELETE | `/projetos/:projetoId/referencias/:referenciaId` | Remove da lista                                             | integrantes do projeto |
| GET    | `/materiais`                             | Materiais, do mais novo ao mais antigo; o aluno recebe os gerais (`cursoId` null) e os da turma em que está pré-autorizado | qualquer autenticado |
| POST   | `/materiais`                             | Publica (`{ titulo, descricao, tipo, url \| arquivo, cursoId }`); `tipo` DOCUMENTO/MODELO/VIDEO/LINK; um link http(s) **ou** a `chave` de `POST /materiais/arquivos` | ADMIN, ORIENTADOR |
| DELETE | `/materiais/:materialId`                 | Remove e apaga o arquivo; o orientador só remove os que publicou   | ADMIN, ORIENTADOR (autor) |
| POST   | `/materiais/arquivos`                    | Pede o envio de um arquivo (`{ nome, tamanho }`), como nas entregas | ADMIN, ORIENTADOR |
| GET    | `/materiais/:materialId/arquivo`         | `{ url, nome }`: link de download de 5 minutos (404 para material de outra turma) | qualquer autenticado |

## Decisões importantes

- **Monólito modular.** Uma Lambda e um deploy, com fronteiras internas:
  cada módulo é dono dos seus dados e regras, e os outros só o enxergam pelas
  interfaces que eles mesmos declaram no `ports.go`. Microsserviços seriam
  infraestrutura e latência de rede sem ganho pro tamanho do projeto.
- **O perfil exigido fica na rota.** Cada `http.go` declara quem pode chamar
  (`admin(...)`, `staff(...)`, `student(...)`, sobre `httpapi.Restrict`), em
  vez de cada handler conferir à mão. Regras que dependem do recurso (ser
  integrante do projeto) ficam no serviço, em `groups.IsMember`/`CanView`.
- **Arquivo vai direto do navegador ao S3.** O back só assina: um POST cuja
  policy fixa a chave, o Content-Type (decidido pela extensão) e o tamanho
  (até 50 MB) — o próprio S3 recusa o resto — e GETs de 5 minutos para baixar.
  Nada passa pela Lambda (limite de 6 MB, e cada MB seria tempo cobrado). A
  entrega só aceita uma chave do próprio grupo/atividade/campo que exista no
  bucket; trocar o arquivo ou devolver a entrega apaga o anterior. Ficam para
  trás (custo baixo) arquivos enviados e nunca entregues e os de projeto ou
  atividade apagados.
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
  exige mudança de Terraform, só registrar no `http.go` do módulo. O router
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
go test ./...      # unitários: serviços com fakes em memória, sem AWS

# ponta a ponta: precisa do DynamoDB Local rodando (porta 8000 por padrão, ver
# https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/DynamoDBLocal.html)
scripts/test-integration.sh
DYNAMODB_LOCAL=http://localhost:8765 scripts/test-integration.sh   # outra porta

# contra as APIs reais do Crossref/OpenAlex (precisa de internet)
REFERENCIAS_INTEGRACAO=1 go test ./internal/references -run Live -v
```

- **Unitários** (`*_test.go` de cada módulo): regras de negócio do serviço,
  com o store e os outros módulos trocados por fakes em memória.
- **Fronteiras** (`internal/app/boundaries_test.go`): falha se um pacote
  importar algo fora das regras — a `platform` não conhece os módulos e um
  módulo só depende de outro quando a dependência está listada ali
  (hoje: `deliveries` e `references` usam `groups`).
- **Contrato** (`internal/integracao/contrato_test.go`): percorre as 65 rotas,
  com casos de sucesso e de erro, e compara status e corpo de cada resposta com
  `testdata/contrato.golden` (ids e datas normalizados). Qualquer mudança na API
  quebra o teste; se a mudança for intencional, regrave e revise o diff:
  `scripts/test-integration.sh -run Contrato -update`.
- **Regras com banco** (`internal/integracao/integracao_test.go`): unicidades,
  um projeto por aluno com cinco inclusões simultâneas, cascata ao apagar.
- **Arquivos** (`internal/integracao/arquivos_test.go`): upload pelo formulário
  assinado num S3 falso que confere a policy, entrega, download, troca e
  limpeza; material de turma invisível para outra turma.
- **Permissões da Lambda** (`internal/integracao/iam_test.go`): registra cada
  operação que o backend faz no DynamoDB, no Cognito e no S3 durante os testes e falha
  se alguma não estiver na política da Lambda (cópia da lista de
  `PFC.Infra/modules/backend-lambda/data.tf`). O DynamoDB Local não confere
  permissão: foi assim que `ConditionCheckItem` e `BatchGetItem` chegaram à AWS
  sem permissão e viraram 500. Ação nova entra nos dois lugares.

O pacote `internal/integracao` usa o DynamoDB Local de verdade e servidores
falsos para o Cognito (`cognito_fake_test.go`), o S3 (`s3_fake_test.go`) e
Crossref/OpenAlex. Sem o
script, ele é pulado.

## Migração

Tabela criada antes da modelagem atual (sem GSI nas turmas, programas e
matrículas, sem travas nem vínculos) precisa passar uma vez pelo
`cmd/migrar`. É idempotente e, sem `-aplicar`, só simula:

```bash
TABLE_NAME=<tabela> go run ./cmd/migrar            # mostra o que faria
TABLE_NAME=<tabela> go run ./cmd/migrar -aplicar
```

A migração também cria os **cursos** a partir das turmas que vieram antes do
cadastro de cursos (com a grafia mais usada entre elas), liga cada turma ao
curso pelo id, conta as turmas no curso e troca a trava da turma pela do
formato novo. Até ela rodar, as turmas antigas continuam funcionando: editar
uma delas já a liga ao curso.

Conflitos (duas turmas iguais, dois programas na mesma turma, um aluno em
dois projetos) aparecem como `CONFLITO:` e precisam de correção manual —
a migração não escolhe qual lado fica. Programa de turma que não existe mais
e período fora do formato aparecem como `AVISO:`.
