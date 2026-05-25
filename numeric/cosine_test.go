package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCosineSparseMaps(t *testing.T) {
	Convey("CosineSparseMaps", t, func() {
		m := map[string]float64{"a": 1, "b": 2}
		So(CosineSparseMaps(m, m), ShouldAlmostEqual, 1, 1e-9)
		So(CosineSparseMaps(map[string]float64{"a": 1}, map[string]float64{"b": 1}), ShouldEqual, 0)
	})
}

func TestCharacterNgramCosine(t *testing.T) {
	Convey("CharacterNgramCosine", t, func() {
		sim, err := CharacterNgramCosine("cab", "cab", 2)
		So(err, ShouldBeNil)
		So(sim, ShouldEqual, 1)
	})

	Convey("Unicode falls back to rune n-grams", t, func() {
		sim, err := CharacterNgramCosine("café", "café", 2)
		So(err, ShouldBeNil)
		So(sim, ShouldAlmostEqual, 1, 1e-9)
	})
}

func BenchmarkCosineSparseMaps(b *testing.B) {
	left := map[string]float64{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
	right := map[string]float64{"a": 2, "b": 1, "c": 4, "d": 3, "f": 1}

	b.ResetTimer()

	for b.Loop() {
		_ = CosineSparseMaps(left, right)
	}
}

func BenchmarkCharacterNgramCosine(b *testing.B) {
	const left = "the_quick_brown_fox"
	const right = "the_lazy_dog_jumps"

	b.ResetTimer()

	for b.Loop() {
		_, _ = CharacterNgramCosine(left, right, 2)
	}
}
