package equation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestCategoryClassify(t *testing.T) {
	Convey("Given a strength vector in the sample slots", t, func() {
		Convey("the equation wires Argmax, EvidenceShare, and ShannonAmbiguity", func() {
			output := types.Frame{}
			output.Put(types.MustSampleSymbol(0), 0.9)
			output.Put(types.MustSampleSymbol(1), 0.2)
			output.Put(types.MustSampleSymbol(2), 0.1)

			types.Step(CategoryClassify(), &output)

			So(output.Err, ShouldBeNil)
			So(output.MustGet(probability.SymbolWinner), ShouldEqual, 0)
			So(output.MustGet(probability.SymbolConfidence), ShouldBeGreaterThan, 0)
			So(output.MustGet(probability.SymbolConfidence), ShouldBeLessThan, 1)
			So(output.MustGet(probability.SymbolAmbiguity), ShouldBeGreaterThan, 0)
		})

		Convey("uniform strengths yield 1/K confidence and full ambiguity", func() {
			output := types.Frame{}
			output.Put(types.MustSampleSymbol(0), 1)
			output.Put(types.MustSampleSymbol(1), 1)
			output.Put(types.MustSampleSymbol(2), 1)

			types.Step(CategoryClassify(), &output)

			So(output.Err, ShouldBeNil)
			So(output.MustGet(probability.SymbolConfidence), ShouldAlmostEqual, 1.0/3.0)
			So(output.MustGet(probability.SymbolAmbiguity), ShouldAlmostEqual, 1.0)
		})
	})
}
