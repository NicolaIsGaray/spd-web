package store

import (
	"slices"
	"testing"
)

// La conversión no necesita base de datos: el formato textual de los arrays es de PostgreSQL.
func TestArrayRoundTrip(t *testing.T) {
	texts := []Array[string]{
		{"admin", "presentador"},
		{`con "comillas"`, "con, coma", "con espacios", `con \ barra`, "{llaves}", "NULL", "ñandú"},
		{},
	}
	for _, in := range texts {
		v, err := in.Value()
		if err != nil {
			t.Fatalf("Value(%q): %v", in, err)
		}
		var out Array[string]
		if err := out.Scan(v); err != nil {
			t.Fatalf("Scan(%v): %v", v, err)
		}
		if !slices.Equal(in, out) {
			t.Fatalf("ida y vuelta: %q → %v → %q", in, v, out)
		}
	}

	ints := Array[int32]{3, 1, 2}
	v, err := ints.Value()
	if err != nil || v != "{3,1,2}" {
		t.Fatalf("Value(%v) = %v, %v", ints, v, err)
	}
	var out Array[int32]
	if err := out.Scan([]byte("{3,1,2}")); err != nil || !slices.Equal(out, ints) {
		t.Fatalf("Scan = %v, %v", out, err)
	}
}

// nil nunca llega a la base de datos como NULL (las columnas son NOT NULL) y un NULL se lee como
// un array vacío, que en JSON es [] y no null.
func TestArrayNilIsEmpty(t *testing.T) {
	if v, err := Array[string](nil).Value(); err != nil || v != "{}" {
		t.Fatalf("Value(nil) = %v, %v; se esperaba {}", v, err)
	}
	var out Array[int32]
	if err := out.Scan(nil); err != nil || out == nil || len(out) != 0 {
		t.Fatalf("Scan(NULL) = %#v, %v; se esperaba un array vacío no nil", out, err)
	}
}
