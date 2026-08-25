package probability

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/types"
)

func frameWith(t *testing.T, samples ...float64) types.Frame {
	t.Helper()

	input := types.Frame{}

	for index, value := range samples {
		input.Put(types.MustSampleSymbol(index), value)
	}

	return input
}

func TestArgmax(t *testing.T) {
	Convey("Given a strength vector in the sample slots", t, func() {
		Convey("Argmax selects the strongest slot", func() {
			output := types.Step(Argmax(), frameWith(t, 0.1, 0.9, 0.4))
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolWinner), ShouldEqual, 1)
		})

		Convey("ties resolve to the lowest index deterministically", func() {
			output := types.Step(Argmax(), frameWith(t, 0.5, 0.5, 0.5))
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolWinner), ShouldEqual, 0)
		})

		Convey("empty input errors", func() {
			output := types.Step(Argmax(), types.Frame{})
			So(output.Err, ShouldNotBeNil)
		})
	})
}

func TestEvidenceShare(t *testing.T) {
	Convey("Given category strengths in the sample slots", t, func() {
		Convey("a lone supported category gets the symmetric-prior share", func() {
			output := types.Step(EvidenceShare(), frameWith(t, 1, 0, 0))
			So(output.Err, ShouldBeNil)
			// (1 + 1) / (1 + 3) = 0.5
			So(output.MustGet(SymbolConfidence), ShouldAlmostEqual, 0.5)
		})

		Convey("equal strengths yield 1/K", func() {
			output := types.Step(EvidenceShare(), frameWith(t, 1, 1, 1))
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolConfidence), ShouldAlmostEqual, 1.0/3.0)
		})

		Convey("a category cannot reach 1.0 from finite evidence alone", func() {
			output := types.Step(EvidenceShare(), frameWith(t, 1000, 1, 1))
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolConfidence), ShouldBeLessThan, 1.0)
		})

		Convey("all-zero evidence yields uniform share", func() {
			output := types.Step(EvidenceShare(), frameWith(t, 0, 0, 0))
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolConfidence), ShouldAlmostEqual, 1.0/3.0)
		})

		Convey("empty input errors", func() {
			output := types.Step(EvidenceShare(), types.Frame{})
			So(output.Err, ShouldNotBeNil)
		})
	})
}

func TestShannonAmbiguity(t *testing.T) {
	Convey("Given a probability distribution in the sample slots", t, func() {
		Convey("uniform distribution is maximally ambiguous", func() {
			output := types.Step(ShannonAmbiguity(), frameWith(t, 0.5, 0.5))
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolAmbiguity), ShouldAlmostEqual, 1.0)
		})

		Convey("concentrated distribution is minimally ambiguous", func() {
			output := types.Step(ShannonAmbiguity(), frameWith(t, 0.9999, 0.0001))
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolAmbiguity), ShouldBeLessThan, 0.01)
		})

		Convey("single category is zero ambiguity", func() {
			output := types.Step(ShannonAmbiguity(), frameWith(t, 1.0))
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolAmbiguity), ShouldEqual, 0)
		})

		Convey("strengths are normalized before measuring entropy", func() {
			output := types.Step(ShannonAmbiguity(), frameWith(t, 4, 4))
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolAmbiguity), ShouldAlmostEqual, 1.0)
		})

		Convey("empty input errors", func() {
			output := types.Step(ShannonAmbiguity(), types.Frame{})
			So(output.Err, ShouldNotBeNil)
		})
	})
}
