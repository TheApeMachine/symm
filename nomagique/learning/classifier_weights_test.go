package learning_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
)

var (
	classifierOutputs = []string{"ignition", "compression", "trend", "exhaustion"}
	classifierTerms   = map[string][]string{
		"ignition":    {"rvol", "precursor"},
		"compression": {"compression", "precursor"},
		"trend":       {"precursor", "compression", "rvol"},
		"exhaustion":  {"rvol", "precursor"},
	}
	classifierInverts = map[string][]string{
		"compression": {"precursor"},
		"trend":       {"compression"},
		"exhaustion":  {"rvol", "precursor"},
	}
	classifierScales = map[string]float64{
		"rvol":        2.0,
		"precursor":   0.1,
		"compression": 1.5,
	}
)

func newClassifier(scales map[string]float64) core.Primitive {
	return learning.NewClassifierWeights(2.0, scales, classifierOutputs, classifierTerms, classifierInverts)
}

func TestClassifierWeightsScores(testingTB *testing.T) {
	Convey("Given typed output recipes", testingTB, func() {
		node := newClassifier(classifierScales)
		So(node.Error(), ShouldBeNil)

		scores := data.Read[[]float64](node.Next(data.NewValue(map[string]float64{
			"rvol":        2.0,
			"precursor":   0.1,
			"compression": 1.5,
		})))

		Convey("It should produce configured logits", func() {
			So(scores, ShouldHaveLength, 4)
			So(scores[0], ShouldBeGreaterThan, 0)
		})
	})
}

func TestClassifierWeightsNegativeFeatures(testingTB *testing.T) {
	Convey("Given negative feature values", testingTB, func() {
		node := newClassifier(classifierScales)
		So(node.Error(), ShouldBeNil)

		negative := data.Read[[]float64](node.Next(data.NewValue(map[string]float64{
			"rvol":        -2.0,
			"precursor":   -0.1,
			"compression": -1.5,
		})))
		zero := data.Read[[]float64](node.Next(data.NewValue(map[string]float64{
			"rvol":        0,
			"precursor":   0,
			"compression": 0,
		})))

		Convey("It should preserve negative contribution instead of squashing to zero", func() {
			So(negative[0], ShouldBeLessThan, 0)
			So(negative[0], ShouldNotEqual, zero[0])
		})
	})
}

func TestNewClassifierWeightsInvalidScale(testingTB *testing.T) {
	Convey("Given a non-positive feature scale", testingTB, func() {
		node := newClassifier(map[string]float64{})

		Convey("It should record the failure and yield nothing", func() {
			So(node.Error(), ShouldNotBeNil)

			for range node.Next(data.NewValue(map[string]float64{})) {
				testingTB.Fatal("invalid classifier weights must yield nothing")
			}
		})
	})
}
