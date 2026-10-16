package common

import (
	"errors"
	"fmt"
	"testing"
)

func TestFalhouItem(t *testing.T) {
	err := fmt.Errorf("salvar: %w", &ErroDeCondicao{Posicoes: []int{0, 2}})
	for i, want := range []bool{true, false, true, false} {
		if got := FalhouItem(err, i); got != want {
			t.Errorf("FalhouItem(err, %d) = %v, queria %v", i, got, want)
		}
	}
	if FalhouItem(errors.New("outro erro"), 0) || FalhouItem(nil, 0) {
		t.Error("erro que não é de condição não pode contar como falha de item")
	}
}
