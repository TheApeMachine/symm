package learning

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLogitScoresMeasure(testingTB *testing.T) {
	Convey("Given typed logit config", testingTB, func() {
		stage, err := NewLogitScores(LogitScoresConfig{
			Weights:   testClassifierConfig(),
			Threshold: 2.0,
			Scales:    testClassifierScales(),
		})
		So(err, ShouldBeNil)

		output, err := stage.Measure(LogitScoresInput{
			Features: map[string]float64{
				"rvol":        2.0,
				"precursor":   0.1,
				"compression": 1.5,
			},
		})

		Convey("It should publish configured classifier logits", func() {
			So(err, ShouldBeNil)
			So(len(output.Scores), ShouldEqual, 4)
			So(output.ByName["ignition"], ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given non-finite feature input", testingTB, func() {
		stage, err := NewLogitScores(LogitScoresConfig{
			Weights:   testClassifierConfig(),
			Threshold: 2.0,
			Scales:    testClassifierScales(),
		})
		So(err, ShouldBeNil)

		_, err = stage.Measure(LogitScoresInput{
			Features: map[string]float64{"rvol": math.NaN()},
		})

		Convey("It should return a validation error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkLogitScoresMeasure(testingTB *testing.B) {
	stage, err := NewLogitScores(LogitScoresConfig{
		Weights:   testClassifierConfig(),
		Threshold: 2.0,
		Scales:    testClassifierScales(),
	})

	if err != nil {
		testingTB.Fatal(err)
	}

	input := LogitScoresInput{
		Features: map[string]float64{
			"rvol":        2.0,
			"precursor":   0.1,
			"compression": 1.5,
		},
	}

	testingTB.ReportAllocs()

	for testingTB.Loop() {
		_, _ = stage.Measure(input)
	}
}
