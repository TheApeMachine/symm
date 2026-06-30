package logic

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/statutil"
	balancefixtures "github.com/theapemachine/symm/tests/fixtures/balances"
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

		ordering, err := left.Compare(measurements, nil, right)

		Convey("It should rank the matching category above the mismatch", func() {
			So(err, ShouldBeNil)
			So(ordering, ShouldBeGreaterThan, 0)
		})
	})
}

func TestConditionOperandHoldingUsesBalancesArtifact(t *testing.T) {
	Convey("Given a balances artifact from the Kraken fixture", t, func() {
		fixture := balancefixtures.NewFixture(balancefixtures.SNAPSHOT, 1)
		var balances *datura.Artifact

		for artifact := range fixture.Artifacts() {
			balances = artifact
			break
		}

		measurements := []*datura.Artifact{
			testMeasurementArtifact(SourceFluid, "BTC/USD", CategoryLaminar, 0.8, 1.0),
		}
		holding := ConditionOperand{
			Type:    SubjectHolding,
			Holding: &HoldingRef{Held: true},
		}

		value, err := holding.resolve(measurements, balances)

		Convey("It should read the held asset directly from the artifact payload", func() {
			So(err, ShouldBeNil)
			So(value, ShouldBeGreaterThan, 0)
		})
	})
}

func TestConditionOperandHoldingFailsClosedWithoutBalancesData(t *testing.T) {
	Convey("Given an empty balances artifact", t, func() {
		balances := datura.Acquire("test", datura.APPJSON).WithRole("balances")
		measurements := []*datura.Artifact{
			testMeasurementArtifact(SourceFluid, "BTC/USD", CategoryLaminar, 0.8, 1.0),
		}
		condition := Condition{
			Type: ConditionIsTrue,
			Left: ConditionOperand{
				Type:    SubjectHolding,
				Holding: &HoldingRef{Held: true},
			},
		}

		matched, err := condition.Evaluate(measurements, balances)

		Convey("It should treat missing ledger evidence as unmet, not fatal", func() {
			So(err, ShouldBeNil)
			So(matched, ShouldBeFalse)
		})
	})
}

func TestConditionOperandCategoryUsesWinningRegime(t *testing.T) {
	Convey("Given a measurement with non-winning category mass", t, func() {
		measurement := testMeasurementArtifact(
			SourceToxicity,
			"BTC/EUR",
			CategoryHardSupport,
			0.8,
			1.0,
		)
		measurement.MergeOutput(
			fmt.Sprintf("category.%d", CategoryIndex(CategoryToxicBluff)),
			0.2,
		)
		measurements := []*datura.Artifact{measurement}

		bluff := ConditionOperand{
			Type:     SubjectCategory,
			Source:   SourceToxicity,
			Category: NewCategory(CategoryToxicBluff),
		}
		hardSupport := ConditionOperand{
			Type:     SubjectCategory,
			Source:   SourceToxicity,
			Category: NewCategory(CategoryHardSupport),
		}

		bluffValue, bluffErr := bluff.resolve(measurements, nil)
		supportValue, supportErr := hardSupport.resolve(measurements, nil)

		Convey("It should only treat the winning category as true", func() {
			So(bluffErr, ShouldBeNil)
			So(supportErr, ShouldBeNil)
			So(bluffValue, ShouldBeLessThan, 0)
			So(supportValue, ShouldBeGreaterThan, 0)
		})
	})
}

func TestConditionOperandCategoryRequiresConfidence(t *testing.T) {
	Convey("Given a zero-confidence winning category", t, func() {
		measurement := testMeasurementArtifact(
			SourceExhaustion,
			"BTC/EUR",
			CategoryMechanicalCollapse,
			0,
			0,
		)
		measurements := []*datura.Artifact{measurement}
		collapse := Condition{
			Type: ConditionIsFalse,
			Left: ConditionOperand{
				Type:     SubjectCategory,
				Source:   SourceExhaustion,
				Category: NewCategory(CategoryMechanicalCollapse),
			},
		}

		matched, err := collapse.Evaluate(measurements, nil)

		Convey("It should not veto on an unconfident category label", func() {
			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)
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
