package advisor

import (
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestNewBasis(t *testing.T) {
	Convey("Given the declarative Basis Advisor", t, func() {
		basis := NewBasis()

		Convey("it exposes one Feature for each basis regime", func() {
			So(basis.Features, ShouldHaveLength, 5)
			So(basis.Features[0].Class.Label, ShouldEqual, "LeverageSqueeze")
			So(basis.Features[1].Class.Label, ShouldEqual, "PremiumExpanding")
			So(basis.Features[2].Class.Label, ShouldEqual, "DiscountExpanding")
			So(basis.Features[3].Class.Label, ShouldEqual, "LiquidationsCascading")
			So(basis.Features[4].Class.Label, ShouldEqual, "NeutralBasis")
		})

		Convey("every selector is qualified and unique inside its Feature", func() {
			for _, feature := range basis.Features {
				So(feature.Clock, ShouldEqual, basisClock)
				seen := make(map[string]bool, len(feature.Keys))

				for _, key := range feature.Keys {
					So(strings.Count(key, "/"), ShouldEqual, 1)
					So(seen[key], ShouldBeFalse)
					seen[key] = true
				}
			}
		})

		Convey("every phase declares a next-bar falsifiable observation", func() {
			expected := []struct {
				label      string
				support    PredictedMove
				contradict PredictedMove
			}{
				{"derivatives/basis_velocity", INCREASE, DECREASE},
				{"derivatives/derivative_spot_log_basis", EXPAND, DISSOLVE},
				{"derivatives/derivative_spot_log_basis", DISSOLVE, EXPAND},
				{"derivatives/gross_liquidation_notional", INCREASE, DECREASE},
				{"derivatives/basis_change", STAGNATE, EXPAND},
			}

			for index, feature := range basis.Features {
				So(feature.Class.Within, ShouldEqual, basisPredictionHorizon)
				So(feature.Class.Predictions, ShouldHaveLength, 1)
				prediction := feature.Class.Predictions[0]
				So(prediction.Support.Label, ShouldEqual, expected[index].label)
				So(prediction.Support.Move, ShouldEqual, expected[index].support)
				So(prediction.Support.Unit, ShouldEqual, RAW)
				So(prediction.Contradict.Label, ShouldEqual, expected[index].label)
				So(prediction.Contradict.Move, ShouldEqual, expected[index].contradict)
				So(prediction.Contradict.Unit, ShouldEqual, RAW)
			}
		})

		Convey("the LeverageSqueeze observation survives the real Basis Arena", func() {
			node := &arenaNode{}
			arena, err := NewArena(BasisName, basis.Features, node, 1)
			So(err, ShouldBeNil)
			issued := basisEnvelope(1, 10.0)
			issued.Perspectives = []*types.Perspective{basisPerspective()}
			So(arena.Step(issued), ShouldNotBeNil)

			supported := basisEnvelope(2, 25.0)
			So(arena.Step(supported), ShouldEqual, supported)
			So(node.perspectives, ShouldHaveLength, 1)
			So(node.perspectives[0].Lifecycle, ShouldEqual, types.PerspectiveSurvived)
			So(node.perspectives[0].ResolvedBy, ShouldEqual,
				types.PerspectiveEvent("derivatives/basis_velocity"))
		})
	})
}

func basisPerspective() *types.Perspective {
	return &types.Perspective{
		Symbol:   "BTC/USD",
		Advisor:  BasisName,
		Question: types.PerspectiveQuestion(BasisName),
		Classes: []types.PerspectiveClass{
			{State: "LeverageSqueeze", Probability: 0.7},
			{State: "PremiumExpanding", Probability: 0.1},
			{State: "DiscountExpanding", Probability: 0.1},
			{State: "LiquidationsCascading", Probability: 0.05},
			{State: "NeutralBasis", Probability: 0.05},
		},
		Lease: types.PerspectiveLease{
			Clock: basisClock,
			From:  1,
			Until: 2,
		},
		Round:     1,
		Lifecycle: types.PerspectiveIssued,
	}
}

func basisEnvelope(ordinal uint64, basisVelocity float64) *types.Envelope {
	at := time.Unix(1_700_000_000+int64(ordinal), 0)
	pumpMeasurement := data.NewMeasurement[float64](
		"pumpdump:BTC/USD",
		"BTC/USD",
		"pumpdump",
		at,
		at,
	)
	pumpMeasurement.PutMetric(data.NewMetric(
		"completed_volume_bar_ordinal",
		float64(ordinal),
		nil,
		nil,
		data.UnitCount,
		data.TimescaleInstantaneous,
	))

	derivMeasurement := data.NewMeasurement[float64](
		"derivatives:BTC/USD",
		"BTC/USD",
		"derivatives",
		at,
		at,
	)
	derivMeasurement.PutMetric(data.NewMetric(
		"basis_velocity",
		basisVelocity,
		nil,
		nil,
		data.UnitRate,
		data.TimescaleInstantaneous,
	))

	envelope := types.NewEnvelope(types.EnvelopeFuturesTicker)
	envelope.FuturesTickerData.Symbol = "BTC/USD"
	envelope.FuturesTickerData.Timestamp = at
	envelope.PumpDump = pumpMeasurement
	envelope.Derivatives = derivMeasurement

	return envelope
}
