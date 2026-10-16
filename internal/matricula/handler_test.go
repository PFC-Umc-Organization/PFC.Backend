package matricula

import (
	"reflect"
	"testing"
)

func TestSepararPorFormato(t *testing.T) {
	validos, invalidos := separarPorFormato([]string{"11221100", "12a45", "998877", "STUDENT#1"})

	if want := []string{"11221100", "998877"}; !reflect.DeepEqual(validos, want) {
		t.Errorf("validos = %v, quer %v", validos, want)
	}
	if len(invalidos) != 2 || invalidos[0].RGM != "12a45" || invalidos[1].RGM != "STUDENT#1" {
		t.Errorf("invalidos = %+v", invalidos)
	}
}
