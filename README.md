# PFC.Backend

Backend em Go do PFC Manager — um único Lambda com router interno atrás de
uma API Gateway REST API.

## Estrutura

```
main.go                    # entrypoint do Lambda, registra as rotas
internal/
├── router/                # roteamento método+path → handler, sem framework
├── common/                # helpers de resposta HTTP (JSON + CORS)
├── auth/                  # domínio de autenticação (ponte com o Cognito)
│   ├── models.go          # espelha os tipos TS do frontend
│   ├── cognito.go          # chamadas ao Cognito (InitiateAuth, SignUp)
│   └── handler.go          # handlers HTTP: POST /auth/login, /auth/registrar
├── matricula/              # allowlist de RGMs (provisionamento pelo admin)
│   ├── models.go           # request/response do lote de RGMs
│   ├── dynamo.go            # BatchWriteItem/Delete na tabela de allowlist
│   └── handler.go            # handlers HTTP: POST/DELETE /admin/students
├── programa/               # Programa de PFC — referencia um Curso (cursoId)
│   ├── models.go
│   ├── dynamo.go            # PK PROGRAM#id — Scan aceitável (baixo volume)
│   └── handler.go            # handlers HTTP: POST/GET /programas
└── projeto/                # Projeto vinculado a um Programa, com orientador
    ├── models.go
    ├── dynamo.go            # PK PROJECT#id, GSI1 pra listar por programa
    └── handler.go            # handlers HTTP: /programas/:id/projetos, /projetos/:id/orientador
```

Cada novo domínio (atividade, material, usuário, etc.) ganha seu próprio
pacote em `internal/`, registrado no `main.go` — nenhuma rota existente
precisa mudar.

## Variáveis de ambiente

| Variável            | Descrição                                    |
| ------------------- | --------------------------------------------- |
| `COGNITO_CLIENT_ID` | App Client ID do Cognito (SPA, sem secret)    |
| `TABLE_NAME`        | Nome da tabela DynamoDB                       |
| `GSI_NAME`          | Nome do índice secundário usado por `projeto` (ex: `GSI1`) |

## Rotas implementadas

| Método | Rota                                    | Descrição                                                        | Autorização |
| ------ | ---------------------------------------- | ------------------------------------------------------------------ | ----------- |
| POST   | `/auth/login`                            | Autentica via Cognito, devolve `{ usuario, token }`                | pública |
| POST   | `/auth/registrar`                        | Cadastro self-service de aluno. `perfil` do corpo é ignorado — sempre ALUNO | pública |
| POST   | `/admin/students`                        | Grava RGMs na allowlist (`STUDENT#<rgm>` ACTIVE)                   | COORDENADOR |
| DELETE | `/admin/students`                        | Remove RGMs da allowlist                                            | COORDENADOR |
| POST   | `/programas`                             | Cria Programa (`{ cursoId }`)                                       | COORDENADOR |
| GET    | `/programas`                             | Lista programas                                                     | qualquer autenticado |
| POST   | `/programas/:programaId/projetos`        | Cria projeto vinculado ao programa                                  | COORDENADOR |
| GET    | `/programas/:programaId/projetos`        | Lista projetos do programa                                          | qualquer autenticado |
| PUT    | `/projetos/:projetoId/orientador`        | Associa orientador (`{ orientadorId }`) ao projeto                  | COORDENADOR |

## Decisões importantes

- **`perfil` nunca vem do cliente em `/auth/registrar`.** Endpoint público;
  se o valor enviado no corpo fosse usado pra decidir o perfil, qualquer
  requisição poderia se autodeclarar COORDENADOR. Cadastro público sempre
  cria ALUNO; contas de professor/coordenador só existem via
  `AdminCreateUser`, com `custom:perfil` setado explicitamente por quem
  cria a conta.
- **Perfil vem do atributo customizado `custom:perfil`, não de grupo
  Cognito.** Grupo é mecanismo de infraestrutura (mapeamento de IAM Role,
  usado no harness pra dar credenciais AWS); perfil é dado de negócio (quem
  essa pessoa é). Ver `internal/common/claims.go`.
- **Token não é validado no login.** O IdToken usado pra montar a resposta
  acabou de ser emitido pelo Cognito na mesma chamada — validar assinatura
  importa em rotas protegidas (via authorizer do API Gateway), não aqui.
- **Programa referencia Curso por id, não duplica dados.** `cursoId` é
  opaco pro backend — não validamos contra nenhuma fonte de verdade de
  Curso ainda (isso vive no frontend/outro lugar por enquanth).
- **API Gateway usa dois proxies genéricos** (`/auth/{proxy+}` sem
  authorizer, `/{proxy+}` com Cognito Authorizer) — nenhuma rota nova
  exige mudança de Terraform, só registrar em `main.go`. O router interno
  (`internal/router`) é quem resolve path parameters (`:programaId` etc.).
- **`cursoIds` do Usuario sempre vem vazio por enquanto** — matrícula em
  curso ainda não tem modelagem definida. Ver TODO em `auth/cognito.go`.

## Build

```bash
GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bootstrap main.go
zip function.zip bootstrap
```

Runtime esperado no Lambda: `provided.al2023`, arquitetura `arm64`.
