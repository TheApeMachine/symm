package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCognitiveReadingPriorMass(testingTB *testing.T) {
	Convey("Given a cognitive reading used as a decision prior", testingTB, func() {
		Convey("When the tree and corpus evidence are positive", func() {
			reading := CognitiveReading{
				ClassConfidence:    0.5,
				LookaheadScore:     0,
				LookaheadPaths:     2,
				CorpusMatchCount:   3,
				TopSimilarity:      0.25,
				PredictedReturnBps: 100,
			}

			Convey("Then it should contribute non-negative topdown mass", func() {
				So(reading.PriorMass(), ShouldAlmostEqual, 0.75)
			})
		})

		Convey("When the reading is negative evidence", func() {
			reading := CognitiveReading{
				ClassConfidence:    -0.5,
				LookaheadScore:     0,
				LookaheadPaths:     2,
				CorpusMatchCount:   3,
				TopSimilarity:      0.25,
				PredictedReturnBps: -100,
			}

			Convey("Then it should not produce an invalid negative scale", func() {
				So(reading.PriorMass(), ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkCognitiveReadingPriorMass(benchmarkTB *testing.B) {
	reading := CognitiveReading{
		ClassConfidence:    0.5,
		LookaheadScore:     0.2,
		LookaheadPaths:     2,
		CorpusMatchCount:   3,
		TopSimilarity:      0.25,
		PredictedReturnBps: 100,
	}

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		_ = reading.PriorMass()
	}
}
