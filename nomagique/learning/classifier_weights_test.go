package learning_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
)

func testClassifierConfig() learning.ClassifierWeightsConfig {
	return learning.ClassifierWeightsConfig{
		Outputs: []string{"ignition", "compression", "trend", "exhaustion"},
		Specs: map[string]learning.LogitSpec{
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

/*
score drives one feature map through the classifier weights primitive.
*/
func score(
	node core.Primitive,
	features map[string]float64,
) learning.ClassifierReading {
	evaluation := transport.NewEvaluate(node)
	var reading learning.ClassifierReading

	for out := range evaluation.Next(transport.NewValues(features).Next(nil)) {
		reading = *(*learning.ClassifierReading)(out)
	}

	return reading
}

func TestClassifierWeightsScores(testingTB *testing.T) {
	Convey("Given typed output recipes", testingTB, func() {
		node := learning.NewClassifierWeights(
			testClassifierConfig(), 2.0, testClassifierScales(),
		)
		So(node.Error(), ShouldBeNil)

		reading := score(node, map[string]float64{
			"rvol":        2.0,
			"precursor":   0.1,
			"compression": 1.5,
		})

		Convey("It should produce configured logits", func() {
			So(reading.Scores, ShouldHaveLength, 4)
			So(reading.Scores[0], ShouldBeGreaterThan, 0)
			So(reading.Strength, ShouldEqual, reading.Scores[0])
		})
	})
}

func TestClassifierWeightsNegativeFeatures(testingTB *testing.T) {
	Convey("Given negative feature values", testingTB, func() {
		node := learning.NewClassifierWeights(
			testClassifierConfig(), 2.0, testClassifierScales(),
		)
		So(node.Error(), ShouldBeNil)

		negative := score(node, map[string]float64{
			"rvol":        -2.0,
			"precursor":   -0.1,
			"compression": -1.5,
		})
		zero := score(node, map[string]float64{
			"rvol":        0,
			"precursor":   0,
			"compression": 0,
		})

		Convey("It should preserve negative contribution instead of squashing to zero", func() {
			So(negative.Scores[0], ShouldBeLessThan, 0)
			So(negative.Scores[0], ShouldNotEqual, zero.Scores[0])
		})
	})
}

func TestNewClassifierWeightsInvalidScale(testingTB *testing.T) {
	Convey("Given a non-positive feature scale", testingTB, func() {
		node := learning.NewClassifierWeights(testClassifierConfig(), 2.0, map[string]float64{})

		Convey("It should record the failure and yield nothing", func() {
			So(node.Error(), ShouldNotBeNil)

			for range node.Next(transport.NewValues(
				map[string]float64{},
			).Next(nil)) {
				testingTB.Fatal("invalid classifier weights must yield nothing")
			}
		})
	})
}
