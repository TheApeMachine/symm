package learning

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestConditionToken(t *testing.T) {
	Convey("The same hot quantity distinguishes buildup, stagnation and reversal", t, func() {
		seen := map[uint64]bool{}
		for _, level := range []float64{-1, 0, 1} {
			for _, change := range []float64{-1, 0, 1} {
				token := ConditionToken(1, level, change)
				So(seen[token], ShouldBeFalse)
				seen[token] = true
				So(token, ShouldNotEqual, ConditionToken(2, level, change))
				So(token, ShouldEqual, ConditionToken(1, level*10, change*10))
			}
		}
	})
}

func TestRemapCondition(t *testing.T) {
	Convey("Warmup preserves conditions while quantity registration order changes", t, func() {
		So(RemapCondition(1, 9), ShouldEqual, 9)
		So(RemapCondition(ConditionToken(1, -1, 1), 9), ShouldEqual, ConditionToken(9, -1, 1))
	})
}

func BenchmarkConditionToken(b *testing.B) {
	for b.Loop() {
		ConditionToken(1, -1, 1)
	}
}
