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
			Name:      "and",
			Seed:      true,
			Operation: NewAnd(true),
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
			Name:      "or",
			Seed:      false,
			Operation: NewOr(false),
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
			Name:      "not",
			Seed:      false,
			Operation: NewNot(),
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
			Name:      "greater",
			Seed:      false,
			Operation: NewGreater[float64](),
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
			Name:      "less",
			Seed:      false,
			Operation: NewLess[float64](),
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
			Name:      "less-equal",
			Seed:      false,
			Operation: NewLessEqual[float64](),
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
			Name:      "equal",
			Seed:      false,
			Operation: NewEqual[float64](),
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
			Name:      "isnan",
			Seed:      false,
			Operation: NewIsNaN[float64](),
			Reference: func(_ bool, value float64) bool {
				return math.IsNaN(value)
			},
		},
	)
}

func TestFiniteHolds(t *testing.T) {
	Convey("Holds is the same predicate Next yields, without a streaming run", t, func() {
		op := NewFinite[float64]()
		So(op.Holds(1.5), ShouldBeTrue)
		So(op.Holds(0), ShouldBeTrue)
		So(op.Holds(math.NaN()), ShouldBeFalse)
		So(op.Holds(math.Inf(1)), ShouldBeFalse)
		So(op.Holds(math.Inf(-1)), ShouldBeFalse)
	})
}

func TestFiniteNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, bool]{
			Name:      "finite",
			Seed:      false,
			Operation: NewFinite[float64](),
			Reference: func(_ bool, value float64) bool {
				return !math.IsNaN(value) && !math.IsInf(value, 0)
			},
		},
	)
}

func TestRejectNext(t *testing.T) {
	Convey("Reject consumes a run and records the configured reason", t, func() {
		reason := errors.New("refused")
		op := NewReject[float64](reason)
		out := tests.CollectSeq(op.Next(tests.SliceToSeq([]float64{1, 2, 3})))

		So(len(out), ShouldEqual, 0)
		So(errors.Is(op.Error(), reason), ShouldBeTrue)
		So(errors.Is(op.Error(), core.ErrDomain), ShouldBeFalse)
	})
}
