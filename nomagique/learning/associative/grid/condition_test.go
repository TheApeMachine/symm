package grid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/* tokenOf drives the Condition primitive for one input tuple. */
func tokenOf(t testing.TB, quantity uint64, level, change float64) uint64 {
	t.Helper()
	op := NewCondition()
	input := [3]float64{float64(quantity), level, change}
	token := data.Read[uint64](op.Next(data.NewValue(input)))

	if token == 0 && op.Error() != nil {
		t.Fatal(op.Error())
	}

	return token
}

func TestCondition(t *testing.T) {
	Convey("The same hot quantity distinguishes buildup, stagnation and reversal", t, func() {
		seen := map[uint64]bool{}

		for _, level := range []float64{-1, 0, 1} {
			for _, change := range []float64{-1, 0, 1} {
				token := tokenOf(t, 1, level, change)
				So(seen[token], ShouldBeFalse)
				seen[token] = true
				So(token, ShouldNotEqual, tokenOf(t, 2, level, change))
				So(token, ShouldEqual, tokenOf(t, 1, level*10, change*10))
			}
		}
	})

	Convey("Invalid quantity returns domain error", t, func() {
		op := NewCondition()
		input := [3]float64{0, 1, 1}
		data.Read[uint64](op.Next(data.NewValue(input)))
		So(op.Error(), ShouldNotBeNil)
	})
}

func BenchmarkCondition(b *testing.B) {
	op := NewCondition()
	input := [3]float64{1, -1, 1}

	for b.Loop() {
		token := data.Read[uint64](op.Next(data.NewValue(input)))
		if token == 0 {
			b.Fatal("condition produced zero token")
		}
	}
}

