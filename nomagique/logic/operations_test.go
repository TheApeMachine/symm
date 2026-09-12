package logic

import (
	"errors"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestAndNext(t *testing.T) {
	tests.Check(
		t, tests.Case[bool, bool]{
			Name: "and",
			Seed: true,
			Factory: func() core.Primitive {
				return NewAnd(true)
			},
			Reference: func(held, value bool) bool {
				return held && value
			},
			CustomVectors: [][]bool{
				{true, false},
				{true, true, true},
				{false},
			},
		},
	)
}

func TestOrNext(t *testing.T) {
	tests.Check(
		t, tests.Case[bool, bool]{
			Name: "or",
			Seed: false,
			Factory: func() core.Primitive {
				return NewOr(false)
			},
			Reference: func(held, value bool) bool {
				return held || value
			},
			CustomVectors: [][]bool{
				{false, true},
				{false, false, true},
				{true},
			},
		},
	)
}

func TestNotNext(t *testing.T) {
	tests.Check(
		t, tests.Case[bool, bool]{
			Name: "not",
			Seed: false,
			Factory: func() core.Primitive {
				return NewNot()
			},
			Reference: func(_, value bool) bool {
				return !value
			},
			CustomVectors: [][]bool{
				{false},
				{true, false, true},
			},
		},
	)
}

func TestGreaterNext(t *testing.T) {
	tests.Check(
		t, tests.Case[[2]float64, bool]{
			Name: "greater",
			Seed: false,
			Factory: func() core.Primitive {
				return NewGreater()
			},
			Reference: func(_ bool, pair [2]float64) bool {
				return pair[0] > pair[1]
			},
			CustomVectors: [][][2]float64{
				{{1, 2}},
				{{2, 1}, {5, 5}, {9, 3}},
			},
		},
	)
}

func TestLessNext(t *testing.T) {
	tests.Check(
		t, tests.Case[[2]float64, bool]{
			Name: "less",
			Seed: false,
			Factory: func() core.Primitive {
				return NewLess()
			},
			Reference: func(_ bool, pair [2]float64) bool {
				return pair[0] < pair[1]
			},
			CustomVectors: [][][2]float64{
				{{1, 2}},
				{{2, 1}, {2, 2}},
			},
		},
	)
}

func TestLessEqualNext(t *testing.T) {
	tests.Check(
		t, tests.Case[[2]float64, bool]{
			Name: "less-equal",
			Seed: false,
			Factory: func() core.Primitive {
				return NewLessEqual()
			},
			Reference: func(_ bool, pair [2]float64) bool {
				return pair[0] <= pair[1]
			},
			CustomVectors: [][][2]float64{
				{{2, 2}},
				{{1, 2}, {3, 1}},
			},
		},
	)
}

func TestEqualNext(t *testing.T) {
	tests.Check(
		t, tests.Case[[2]float64, bool]{
			Name: "equal",
			Seed: false,
			Factory: func() core.Primitive {
				return NewEqual()
			},
			Reference: func(_ bool, pair [2]float64) bool {
				return pair[0] == pair[1]
			},
			CustomVectors: [][][2]float64{
				{{2, 2}},
				{{1, 2}, {3, 3}},
			},
		},
	)
}

func TestIsNaNNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, bool]{
			Name: "isnan",
			Seed: false,
			Factory: func() core.Primitive {
				return NewIsNaN()
			},
			Reference: func(_ bool, value float64) bool {
				return math.IsNaN(value)
			},
		},
	)
}

func TestIsInfNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, bool]{
			Name: "isinf",
			Seed: false,
			Factory: func() core.Primitive {
				return NewIsInf()
			},
			Reference: func(_ bool, value float64) bool {
				return math.IsInf(value, 0)
			},
		},
	)
}

func TestFiniteNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, bool]{
			Name: "finite",
			Seed: false,
			Factory: func() core.Primitive {
				return NewFinite()
			},
			Reference: func(_ bool, value float64) bool {
				return !math.IsNaN(value) && !math.IsInf(value, 0)
			},
		},
	)
}

func TestRejectNext(t *testing.T) {
	Convey("Reject consumes a run and records the configured reason", t, func() {
		reason := errors.New("refused")
		op := NewReject(reason)
		out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq([]float64{1, 2, 3})))

		So(len(out), ShouldEqual, 0)
		So(errors.Is(op.Error(), reason), ShouldBeTrue)
		So(errors.Is(op.Error(), core.ErrDomain), ShouldBeFalse)
	})
}

func TestGateNext(t *testing.T) {
	Convey("Gate routes each arrival through pass or fail based on predicate", t, func() {
		// Predicate: Finite()
		// Pass: Not()
		// Fail: Not()
		gate := NewGate(NewFinite(), NewNot(), NewNot())
		in := tests.SliceToSeq([]bool{true, false})
		out := tests.CollectSeq[bool](gate.Next(in))

		So(out, ShouldResemble, []bool{false, true})
		So(gate.Error(), ShouldBeNil)
	})
}

func TestPickNext(t *testing.T) {
	Convey("Pick selects candidates according to predicate", t, func() {
		// Greater picks larger value (running maximum)
		pick := NewPick(NewGreater())
		in := tests.SliceToSeq([]float64{3.0, 1.0, 5.0, 2.0})
		out := tests.CollectSeq[float64](pick.Next(in))

		So(out, ShouldResemble, []float64{3.0, 3.0, 5.0, 5.0})
		So(pick.Error(), ShouldBeNil)
	})
}
