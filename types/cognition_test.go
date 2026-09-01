package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCognitionClone(t *testing.T) {
	Convey("Given a cognition reading backed by mutable model containers", t, func() {
		reading := Cognition{
			Predictions: map[string]float64{"trend": 0.75},
			Branches:    []CognitionBranch{{Key: "trend", Count: 1}},
			Beams:       []CognitionBeam{{Key: "trend", Score: 0.75}},
			Classes:     []CognitionClass{{Name: "trend", Probability: 0.75}},
			Dreams:      []string{"trend"},
		}
		observation := reading.Clone()
		reading.Predictions["trend"] = 0.25
		reading.Branches[0].Count = 2
		reading.Beams[0].Score = 0.25
		reading.Classes[0].Probability = 0.25
		reading.Dreams[0] = "reversal"

		Convey("the event observation remains unchanged", func() {
			So(observation.Predictions["trend"], ShouldEqual, 0.75)
			So(observation.Branches[0].Count, ShouldEqual, 1)
			So(observation.Beams[0].Score, ShouldEqual, 0.75)
			So(observation.Classes[0].Probability, ShouldEqual, 0.75)
			So(observation.Dreams[0], ShouldEqual, "trend")
		})
	})
}

func BenchmarkCognitionClone(b *testing.B) {
	reading := Cognition{
		Predictions: map[string]float64{"trend": 0.75},
		Branches:    make([]CognitionBranch, 128),
		Beams:       make([]CognitionBeam, 8),
		Classes:     make([]CognitionClass, 6),
	}

	b.ReportAllocs()

	for b.Loop() {
		reading.Clone()
	}
}
