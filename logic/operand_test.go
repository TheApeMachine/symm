package logic

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/statutil"
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
	index := CategoryIndex(category)
	artifact.Poke(datura.Map[float64]{
		fmt.Sprintf("category.%d", index): 1,
		"value":                           float64(index),
		"confidence":                      confidence,
		"strength":                        strength,
	}, "output")

	return artifact
}

func TestConditionOperandCompare(t *testing.T) {
	Convey("Given category operands for the same source", t, func() {
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

		Convey("It should rank the matching category above the mismatch", func() {
			So(err, ShouldBeNil)
			So(ordering, ShouldBeGreaterThan, 0)
		})
	})
}

func TestConfidenceBaseline(t *testing.T) {
	Convey("Given cross-section confidences", t, func() {
		measurements := []*datura.Artifact{
			testMeasurementArtifact(SourceFluid, "BTC/EUR", CategoryLaminar, 0.2, 1.0),
			testMeasurementArtifact(SourceCVD, "BTC/EUR", CategoryOrganic, 0.5, 1.0),
			testMeasurementArtifact(SourceHawkes, "BTC/EUR", CategoryFrenzy, 0.8, 1.0),
		}

		entryBaseline, err := confidenceBaseline(measurements, ConfidenceEntryBaseline)

		Convey("It should stabilize entry gates with MAD when the sample is thin", func() {
			So(err, ShouldBeNil)
			So(entryBaseline, ShouldAlmostEqual, 0.8, 1e-12)
		})
	})

	Convey("Given enough cross-section confidences", t, func() {
		measurements := []*datura.Artifact{
			testMeasurementArtifact(SourceFluid, "BTC/EUR", CategoryLaminar, 0.1, 1.0),
			testMeasurementArtifact(SourceCVD, "BTC/EUR", CategoryOrganic, 0.3, 1.0),
			testMeasurementArtifact(SourceHawkes, "BTC/EUR", CategoryFrenzy, 0.5, 1.0),
			testMeasurementArtifact(SourcePumpDump, "BTC/EUR", CategoryVerticalIgnition, 0.7, 1.0),
			testMeasurementArtifact(SourceToxicity, "BTC/EUR", CategoryToxicBluff, 0.9, 1.0),
		}

		_, expectedEntry, err := statutil.Quartiles([]float64{0.1, 0.3, 0.5, 0.7, 0.9})
		So(err, ShouldBeNil)

		entryBaseline, err := confidenceBaseline(measurements, ConfidenceEntryBaseline)

		Convey("It should return the upper quartile once the sample budget is met", func() {
			So(err, ShouldBeNil)
			So(entryBaseline, ShouldAlmostEqual, expectedEntry, 1e-12)
		})
	})
}

func TestConditionOperandEigenmodeUnavailable(t *testing.T) {
	Convey("Given measurements without eigenmode energy", t, func() {
		operand := ConditionOperand{
			Type: SubjectEigenmode,
			Eigenmode: &EigenmodeRef{
				Mode: EigenmodeMomentum,
			},
		}

		value, err := operand.resolve([]*datura.Artifact{}, nil)

		Convey("It should fail closed as unknown evidence", func() {
			So(err, ShouldEqual, errUnknownMeasurement)
			So(value, ShouldEqual, 0)
		})
	})
}
