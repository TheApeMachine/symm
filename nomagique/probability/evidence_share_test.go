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
			output := frameWith(t, 0.1, 0.9, 0.4)
			types.Step(Argmax(), &output)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolWinner), ShouldEqual, 1)
		})

		Convey("ties resolve to the lowest index deterministically", func() {
			output := frameWith(t, 0.5, 0.5, 0.5)
			types.Step(Argmax(), &output)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolWinner), ShouldEqual, 0)
		})

		Convey("empty input errors", func() {
			output := types.Frame{}
			types.Step(Argmax(), &output)
			So(output.Err, ShouldNotBeNil)
		})
	})
}

func TestEvidenceShare(t *testing.T) {
	Convey("Given category strengths in the sample slots", t, func() {
		Convey("a lone supported category gets the symmetric-prior share", func() {
			output := frameWith(t, 1, 0, 0)
			types.Step(EvidenceShare(), &output)
			So(output.Err, ShouldBeNil)
			// (1 + 1) / (1 + 3) = 0.5
			So(output.MustGet(SymbolConfidence), ShouldAlmostEqual, 0.5)
		})

		Convey("an unsupported selected category gets only its pseudocount", func() {
			output := frameWith(t, 1, 0, 0)
			output.Put(SymbolWinner, 1)
			types.Step(EvidenceShare(), &output)
			So(output.Err, ShouldBeNil)
			// (0 + 1) / (1 + 3) = 0.25
			So(output.MustGet(SymbolConfidence), ShouldAlmostEqual, 0.25)
		})

		Convey("equal strengths yield 1/K", func() {
			output := frameWith(t, 1, 1, 1)
			types.Step(EvidenceShare(), &output)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolConfidence), ShouldAlmostEqual, 1.0/3.0)
		})

		Convey("a category cannot reach 1.0 from finite evidence alone", func() {
			output := frameWith(t, 1000, 1, 1)
			types.Step(EvidenceShare(), &output)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolConfidence), ShouldBeLessThan, 1.0)
		})

		Convey("all-zero evidence yields uniform share", func() {
			output := frameWith(t, 0, 0, 0)
			types.Step(EvidenceShare(), &output)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolConfidence), ShouldAlmostEqual, 1.0/3.0)
		})

		Convey("empty input errors", func() {
			output := types.Frame{}
			types.Step(EvidenceShare(), &output)
			So(output.Err, ShouldNotBeNil)
		})
	})
}

func TestShannonAmbiguity(t *testing.T) {
	Convey("Given a probability distribution in the sample slots", t, func() {
		Convey("uniform distribution is maximally ambiguous", func() {
			output := frameWith(t, 0.5, 0.5)
			types.Step(ShannonAmbiguity(), &output)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolAmbiguity), ShouldAlmostEqual, 1.0)
		})

		Convey("concentrated distribution is minimally ambiguous", func() {
			output := frameWith(t, 0.9999, 0.0001)
			types.Step(ShannonAmbiguity(), &output)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolAmbiguity), ShouldBeLessThan, 0.01)
		})

		Convey("single category is zero ambiguity", func() {
			output := frameWith(t, 1.0)
			types.Step(ShannonAmbiguity(), &output)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolAmbiguity), ShouldEqual, 0)
		})

		Convey("strengths are normalized before measuring entropy", func() {
			output := frameWith(t, 4, 4)
			types.Step(ShannonAmbiguity(), &output)
			So(output.Err, ShouldBeNil)
			So(output.MustGet(SymbolAmbiguity), ShouldAlmostEqual, 1.0)
		})

		Convey("empty input errors", func() {
			output := types.Frame{}
			types.Step(ShannonAmbiguity(), &output)
			So(output.Err, ShouldNotBeNil)
		})
	})
}

func BenchmarkEvidenceShare(b *testing.B) {
	template := types.Frame{}
	template.Put(types.MustSampleSymbol(0), 1)
	template.Put(types.MustSampleSymbol(1), 0)
	template.Put(types.MustSampleSymbol(2), 0)
	template.Put(SymbolWinner, 1)
	b.ReportAllocs()

	for b.Loop() {
		output := template
		types.Step(EvidenceShare(), &output)

		if output.Err != nil {
			b.Fatal(output.Err)
		}
	}
}
