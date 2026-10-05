package storage

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
)

func TestNaturalCompareOrdersLikeAHuman(t *testing.T) {
	got := []string{
		"slide10.png", "Diapositiva11.PNG", "slide2.png", "010.png", "Slide1.png",
		"a/slide3.png", "002.png", "Diapositiva9.PNG", "slide02.png", "001.png",
	}
	slices.SortFunc(got, NaturalCompare)
	want := []string{
		"001.png", "002.png", "010.png", "a/slide3.png", "Diapositiva9.PNG",
		"Diapositiva11.PNG", "Slide1.png", "slide2.png", "slide02.png", "slide10.png",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("orden:\n got %q\nwant %q", got, want)
	}
}

// Con nombres rellenados con ceros, el orden natural es exactamente el lexicográfico.
func TestNaturalCompareMatchesLexicographicForPaddedNames(t *testing.T) {
	var names []string
	for i := 1; i <= 250; i++ {
		names = append(names, fmt.Sprintf("%03d.png", i))
	}
	natural := slices.Clone(names)
	rand.Shuffle(len(natural), func(i, j int) { natural[i], natural[j] = natural[j], natural[i] })
	lexicographic := slices.Clone(natural)

	slices.SortFunc(natural, NaturalCompare)
	slices.Sort(lexicographic)
	if !slices.Equal(natural, lexicographic) {
		t.Fatal("el orden natural difiere del lexicográfico con nombres rellenados con ceros")
	}
}

func TestNaturalCompareHugeNumbersDoNotOverflow(t *testing.T) {
	a, b := "s99999999999999999999999.png", "s100000000000000000000000.png"
	if NaturalCompare(a, b) >= 0 {
		t.Fatal("se esperaba a < b")
	}
}
