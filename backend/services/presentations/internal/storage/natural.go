package storage

import (
	"cmp"
	"strings"
)

// NaturalCompare define el orden de las diapositivas: lexicográfico, salvo que los tramos de
// dígitos se comparan por su valor numérico. Con nombres rellenados con ceros (001.png,
// 002.png...) el resultado es idéntico al orden lexicográfico puro; con nombres sin relleno,
// como los que exporta PowerPoint (Diapositiva1.PNG ... Diapositiva10.PNG), evita que
// "Diapositiva10" quede antes que "Diapositiva2". No distingue mayúsculas; a igualdad desempata
// con el orden lexicográfico estricto para que el resultado sea determinista.
func NaturalCompare(a, b string) int {
	if c := naturalCompare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			na, restA := leadingDigits(a)
			nb, restB := leadingDigits(b)
			if c := compareNumeric(na, nb); c != 0 {
				return c
			}
			a, b = restA, restB
			continue
		}
		if a[0] != b[0] {
			return cmp.Compare(a[0], b[0])
		}
		a, b = a[1:], b[1:]
	}
	return cmp.Compare(len(a), len(b))
}

// compareNumeric compara dos tramos de dígitos por valor sin convertirlos a entero (sin overflow).
func compareNumeric(x, y string) int {
	tx, ty := strings.TrimLeft(x, "0"), strings.TrimLeft(y, "0")
	if c := cmp.Compare(len(tx), len(ty)); c != 0 {
		return c
	}
	if c := strings.Compare(tx, ty); c != 0 {
		return c
	}
	return cmp.Compare(len(x), len(y)) // mismo valor: primero el de menos ceros a la izquierda
}

func leadingDigits(s string) (digits, rest string) {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return s[:i], s[i:]
}

func isDigit(b byte) bool { return '0' <= b && b <= '9' }
