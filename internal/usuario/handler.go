package usuario

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"

	"github.com/PFC-Umc-Organization/PFC.Backend/internal/auditoria"
	"github.com/PFC-Umc-Organization/PFC.Backend/internal/common"
)

// HandleListar implementa GET /usuarios.
//
// Qualquer autenticado pode listar: o aluno precisa dos nomes pra ver os
// colegas de grupo e o orientador (tela Meu PFC). Mas e-mail é dado pessoal
// e só a equipe acadêmica recebe — pro aluno ele sai da resposta.
func HandleListar(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	usuarios, err := listarContas(ctx)
	if err != nil {
		// Vai pro CloudWatch — o cliente só recebe a mensagem genérica.
		log.Printf("GET /usuarios: %v", err)
		return common.Erro(500, "falha ao listar usuários"), nil
	}

	if !common.PerfilPermitido(req, common.Admin, common.Orientador) {
		for i := range usuarios {
			usuarios[i].Email = ""
		}
	}
	return common.JSON(200, usuarios), nil
}

// HandleCriarConta implementa POST /admin/usuarios — admin cadastra
// orientador ou outro admin. Aluno nunca entra por aqui (ver NovaConta).
func HandleCriarConta(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !common.PerfilPermitido(req, common.Admin) {
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "usuario.criado", Resultado: "falha", Motivo: "acesso restrito a administradores",
		}, req)
		return common.Erro(403, "acesso restrito a administradores"), nil
	}

	var body NovaConta
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		return common.Erro(400, "corpo da requisição inválido"), nil
	}
	body.Nome = strings.TrimSpace(body.Nome)
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	if body.Nome == "" || body.Email == "" {
		return common.Erro(400, "nome e email são obrigatórios"), nil
	}
	// @umc.br é o domínio institucional de professor/admin — diferente de
	// @alunos.umc.br, que é só de aluno (self sign-up).
	if !strings.HasSuffix(body.Email, "@umc.br") {
		return common.Erro(400, "use um e-mail institucional (@umc.br)"), nil
	}

	perfil := common.Perfil(body.Perfil)
	if perfil != common.Orientador && perfil != common.Admin {
		return common.Erro(400, "perfil deve ser ORIENTADOR ou ADMIN"), nil
	}

	if err := CriarConta(ctx, body.Nome, body.Email, perfil); err != nil {
		log.Printf("POST /admin/usuarios: %v", err)
		status, mensagem := mensagemDeErroAoCriarConta(err)
		auditoria.Registrar(ctx, auditoria.Evento{
			Acao: "usuario.criado", Resultado: "falha", Motivo: mensagem,
			Detalhes: map[string]any{"email": body.Email, "perfil": string(perfil)},
		}, req)
		return common.Erro(status, mensagem), nil
	}

	auditoria.Registrar(ctx, auditoria.Evento{
		Acao: "usuario.criado", Resultado: "sucesso",
		Detalhes: map[string]any{"email": body.Email, "perfil": string(perfil)},
	}, req)

	return common.JSON(201, map[string]string{
		"mensagem": "conta criada — um convite foi enviado por e-mail",
	}), nil
}

// mensagemDeErroAoCriarConta traduz os erros mais comuns de
// AdminCreateUser numa mensagem que diz ao admin o que corrigir.
func mensagemDeErroAoCriarConta(err error) (status int, mensagem string) {
	var (
		existente *types.UsernameExistsException
		parametro *types.InvalidParameterException
	)
	switch {
	case errors.As(err, &existente):
		return 409, "já existe uma conta com esse e-mail"
	case errors.As(err, &parametro):
		return 400, "e-mail inválido"
	default:
		return 500, "falha ao criar a conta"
	}
}
