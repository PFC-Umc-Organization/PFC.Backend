package atividade

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	tamanhoMaxTitulo    = 150
	tamanhoMaxDescricao = 5000
	tamanhoMaxRotulo    = 100
	tamanhoMaxTexto     = 300
	tamanhoMaxTextoLong = 5000
	tamanhoMaxArquivo   = 255
	maxCampos           = 20
)

// layoutsPrazo: o <input type="datetime-local"> do front manda sem fuso
// ("2026-10-15T23:59"); aceita-se também segundos e RFC 3339.
var layoutsPrazo = []string{"2006-01-02T15:04", "2006-01-02T15:04:05", time.RFC3339}

func prazoValido(prazo string) bool {
	for _, l := range layoutsPrazo {
		if _, err := time.Parse(l, prazo); err == nil {
			return true
		}
	}
	return false
}

func tipoValido(t TipoCampo) bool {
	switch t {
	case CampoArquivo, CampoTexto, CampoTextoLongo, CampoLink:
		return true
	}
	return false
}

// dadosAtividade são título, descrição e prazo já aparados.
type dadosAtividade struct {
	Titulo, Descricao, Prazo string
}

// normalizarDados apara e valida título, descrição e prazo. O segundo
// retorno é a mensagem de erro (vazia se ok).
func normalizarDados(titulo, descricao, prazo string) (dadosAtividade, string) {
	d := dadosAtividade{
		Titulo:    strings.TrimSpace(titulo),
		Descricao: strings.TrimSpace(descricao),
		Prazo:     strings.TrimSpace(prazo),
	}
	switch {
	case d.Titulo == "":
		return d, "título é obrigatório"
	case utf8.RuneCountInString(d.Titulo) > tamanhoMaxTitulo:
		return d, "título longo demais"
	case utf8.RuneCountInString(d.Descricao) > tamanhoMaxDescricao:
		return d, "descrição longa demais"
	case !prazoValido(d.Prazo):
		return d, "prazo inválido"
	}
	return d, ""
}

// normalizarCampo apara e valida um campo de entrega.
func normalizarCampo(c NovoCampo) (NovoCampo, string) {
	c.Rotulo = strings.TrimSpace(c.Rotulo)
	switch {
	case c.Rotulo == "":
		return c, "nome do campo é obrigatório"
	case utf8.RuneCountInString(c.Rotulo) > tamanhoMaxRotulo:
		return c, "nome do campo longo demais"
	case !tipoValido(c.Tipo):
		return c, "tipo de campo inválido"
	}
	return c, ""
}

// normalizarCampos valida os campos informados na criação da atividade.
// Lista vazia vira o campo padrão (arquivo obrigatório).
func normalizarCampos(campos []NovoCampo) ([]NovoCampo, string) {
	if len(campos) == 0 {
		return []NovoCampo{{Rotulo: "Arquivo da entrega", Tipo: CampoArquivo, Obrigatorio: true}}, ""
	}
	if len(campos) > maxCampos {
		return nil, "campos demais na atividade"
	}
	limpos := make([]NovoCampo, 0, len(campos))
	for _, c := range campos {
		n, msg := normalizarCampo(c)
		if msg != "" {
			return nil, msg
		}
		limpos = append(limpos, n)
	}
	return limpos, ""
}

// validarRespostas confere as respostas contra os campos da atividade:
// só ids conhecidos, obrigatórios preenchidos, tamanho e formato por tipo.
// Devolve as respostas já aparadas (sem as vazias) ou a mensagem de erro.
func validarRespostas(campos []Campo, respostas map[string]string) (map[string]string, string) {
	conhecidos := make(map[string]bool, len(campos))
	for _, c := range campos {
		conhecidos[c.ID] = true
	}
	for id := range respostas {
		if !conhecidos[id] {
			return nil, "a resposta tem um campo que não existe na atividade"
		}
	}

	limpas := make(map[string]string, len(campos))
	for _, c := range campos {
		valor := strings.TrimSpace(respostas[c.ID])
		if valor == "" {
			if c.Obrigatorio {
				return nil, fmt.Sprintf("o campo %q é obrigatório", c.Rotulo)
			}
			continue
		}

		limite := tamanhoMaxTexto
		switch c.Tipo {
		case CampoTextoLongo:
			limite = tamanhoMaxTextoLong
		case CampoArquivo:
			limite = tamanhoMaxArquivo
		case CampoLink:
			if !strings.HasPrefix(valor, "http://") && !strings.HasPrefix(valor, "https://") {
				return nil, fmt.Sprintf("o campo %q precisa de um link http(s)", c.Rotulo)
			}
		}
		if utf8.RuneCountInString(valor) > limite {
			return nil, fmt.Sprintf("o campo %q passou do tamanho máximo", c.Rotulo)
		}
		limpas[c.ID] = valor
	}
	return limpas, ""
}
