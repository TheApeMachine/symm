package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
)

func TestFeatureDetectorOneLineAPI(t *testing.T) {
	Convey("Given a 1-line instantiated overcomplete feature detector", t, func() {
		// 1. Instantiate in one line
		detector := NewFeatureDetector(FeatureDetectorConfig{
			InputDim:      3,
			DictionaryDim: 12, // 4x overcomplete sparse dictionary
			LatentDim:     4,  // multi-timescale temporal state
			TargetDim:     1,  // online task readout
		})

		Convey("Feeding data directly via Step() extracts features, energy, and surprises", func() {
			// 2. Feed data and read out in one line
			out, err := detector.Step(0.5, -0.2, 0.8)
			So(err, ShouldBeNil)
			So(out.InferenceSteps, ShouldBeGreaterThan, 0)
			So(out.Surprise, ShouldBeGreaterThanOrEqualTo, 0)
			So(len(out.Readout), ShouldEqual, 12+4+3+12) // [z1 (12), z2 (4), e0 (3), e1 (12)]
		})

		Convey("Feeding data via nomagique.Stream operates seamlessly with universal Frames", func() {
			stream := nomagique.NewStream(detector.Primitive(), nomagique.Frame{})

			input := nomagique.Frame{}
			input.Put(SymbolFeatureCount, 3)
			input.Put(FeatureSymbol(0), 0.1)
			input.Put(FeatureSymbol(1), 0.4)
			input.Put(FeatureSymbol(2), -0.3)

			output, err := stream.Step(input)
			So(err, ShouldBeNil)
			So(output.Has(SymbolEnergy), ShouldBeTrue)
			So(output.Has(SymbolSurprise), ShouldBeTrue)
			So(output.MustGet(SymbolLatentCount), ShouldEqual, 16)     // 12 + 4
			So(output.MustGet(SymbolInnovationCount), ShouldEqual, 15) // 3 + 12
		})
	})
}