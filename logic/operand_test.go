package logic

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/statistic"
)

func testMeasurementArtifact(
	source SourceType,
	scope string,
	category CategoryType,
	confidence float64,
	strength float64,
) *datura.Artifact {
	artifact := datura.Acquire("test", datura.Artifact_Type_json)
	artifact.WithRole("measurement")
	artifact.WithScope(scope)
	_ = artifact.SetOrigin(string(source))
	artifact.WithPayload([]byte(`{}`))
	artifact.Poke(datura.Map[float64]{
		"value":      float64(CategoryIndex(category)),
		"confidence": confidence,
		"strength":   strength,
	}, "output")

	return artifact
}

func TestConditionOperandCompare(testingTB *testing.T) {
	convey.Convey("Given category operands for the same source", testingTB, func() {
		measurements := []*datura.Artifact{
			testMeasurementArtifact(
				SourceFluid,
				"BTC/EUR",
				CategoryLaminar,
				0.8,
				1.0,
			),
		}

		left := ConditionOperand{
			Type:     SubjectCategory,
			Source:   SourceFluid,
			Category: NewCategory(CategoryLaminar),
		}
		right := ConditionOperand{
			Type:     SubjectCategory,
			Source:   SourceFluid,
			Category: NewCategory(CategoryTurbulent),
		}

		ordering, err := left.Compare(measurements, &Balances{}, right)

		convey.Convey("It should rank the matching category above the mismatch", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(ordering, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestConfidenceBaseline(testingTB *testing.T) {
	convey.Convey("Given cross-section confidences", testingTB, func() {
		measurements := []*datura.Artifact{
			testMeasurementArtifact(SourceFluid, "BTC/EUR", CategoryLaminar, 0.2, 1.0),
			testMeasurementArtifact(SourceCVD, "BTC/EUR", CategoryOrganic, 0.5, 1.0),
			testMeasurementArtifact(SourceHawkes, "BTC/EUR", CategoryFrenzy, 0.8, 1.0),
		}

		_, expectedEntry := statistic.Quartiles([]float64{0.2, 0.5, 0.8})

		entryBaseline, err := confidenceBaseline(measurements, ConfidenceEntryBaseline)

		convey.Convey("It should return the upper quartile for entry gates", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(entryBaseline, convey.ShouldAlmostEqual, expectedEntry, 1e-12)
		})
	})
}
