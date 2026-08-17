package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func testClassifierConfig() ClassifierWeightsConfig {
	return ClassifierWeightsConfig{
		Outputs: []string{"ignition", "compression", "trend", "exhaustion"},
		Specs: map[string]LogitSpec{
			"ignition": {
				Terms: []string{"rvol", "precursor"},
			},
			"compression": {
				Terms:   []string{"compression", "precursor"},
				Inverts: map[string]bool{"precursor": true},
			},
			"trend": {
				Terms:   []string{"precursor", "compression", "rvol"},
				Inverts: map[string]bool{"compression": true},
			},
			"exhaustion": {
				Terms:   []string{"rvol", "precursor"},
				Inverts: map[string]bool{"rvol": true, "precursor": true},
			},
		},
	}
}

func testClassifierScales() map[string]float64 {
	return map[string]float64{
		"rvol":        2.0,
		"precursor":   0.1,
		"compression": 1.5,
	}
}

func TestClassifierWeightsScores(testingTB *testing.T) {
	Convey("Given typed output recipes", testingTB, func() {
		weights, err := NewClassifierWeights(testClassifierConfig(), 2.0, testClassifierScales())
		So(err, ShouldBeNil)

		scores := weights.Scores(map[string]float64{
			"rvol":        2.0,
			"precursor":   0.1,
			"compression": 1.5,
		})

		Convey("It should produce configured logits", func() {
			So(len(scores), ShouldEqual, 4)
			So(scores[0], ShouldBeGreaterThan, 0)
		})
	})
}

func TestClassifierWeightsNegativeFeatures(testingTB *testing.T) {
	Convey("Given negative feature values", testingTB, func() {
		weights, err := NewClassifierWeights(testClassifierConfig(), 2.0, testClassifierScales())
		So(err, ShouldBeNil)

		negativeScores := weights.Scores(map[string]float64{
			"rvol":        -2.0,
			"precursor":   -0.1,
			"compression": -1.5,
		})
		zeroScores := weights.Scores(map[string]float64{
			"rvol":        0,
			"precursor":   0,
			"compression": 0,
		})

		Convey("It should preserve negative contribution instead of squashing to zero", func() {
			So(negativeScores[0], ShouldBeLessThan, 0)
			So(negativeScores[0], ShouldNotEqual, zeroScores[0])
		})
	})
}

func TestNewClassifierWeightsInvalidScale(testingTB *testing.T) {
	Convey("Given a non-positive feature scale", testingTB, func() {
		_, err := NewClassifierWeights(testClassifierConfig(), 2.0, map[string]float64{})

		Convey("It should return an error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}
