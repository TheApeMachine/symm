package advisor

import (
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestNewLiquidity(t *testing.T) {
	Convey("Given the declarative Liquidity Advisor", t, func() {
		liquidity := NewLiquidity()

		Convey("it exposes one Feature for each liquidity regime", func() {
			So(liquidity.Features, ShouldHaveLength, 5)
			So(liquidity.Features[0].Class.Label, ShouldEqual, "WallBuilding")
			So(liquidity.Features[1].Class.Label, ShouldEqual, "VacuumForming")
			So(liquidity.Features[2].Class.Label, ShouldEqual, "Replenishing")
			So(liquidity.Features[3].Class.Label, ShouldEqual, "Depleting")
			So(liquidity.Features[4].Class.Label, ShouldEqual, "Balanced")
		})

		Convey("every selector is qualified and unique inside its Feature", func() {
			for _, feature := range liquidity.Features {
				So(feature.Clock, ShouldEqual, liquidityClock)
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
				{"liquidity/two_sided_touch_notional", INCREASE, DECREASE},
				{"liquidity/relative_spread", EXPAND, DISSOLVE},
				{"toxicity/net_replenished_quantity:bid", INCREASE, DECREASE},
				{"toxicity/touch_fill_rate:bid", INCREASE, DECREASE},
				{"liquidity/touch_notional_imbalance", STAGNATE, EXPAND},
			}

			for index, feature := range liquidity.Features {
				So(feature.Class.Within, ShouldEqual, liquidityPredictionHorizon)
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

		Convey("the WallBuilding observation survives the real Liquidity Arena", func() {
			node := &arenaNode{}
			arena, err := NewArena(LiquidityName, liquidity.Features, node, 1)
			So(err, ShouldBeNil)
			issued := liquidityEnvelope(1, 1000.0)
			issued.Perspectives = []*types.Perspective{liquidityPerspective()}
			So(arena.Step(issued), ShouldNotBeNil)

			supported := liquidityEnvelope(2, 2500.0)
			So(arena.Step(supported), ShouldEqual, supported)
			So(node.perspectives, ShouldHaveLength, 1)
			So(node.perspectives[0].Lifecycle, ShouldEqual, types.PerspectiveSurvived)
			So(node.perspectives[0].ResolvedBy, ShouldEqual,
				types.PerspectiveEvent("liquidity/two_sided_touch_notional"))
		})
	})
}

func liquidityPerspective() *types.Perspective {
	return &types.Perspective{
		Symbol:   "BTC/USD",
		Advisor:  LiquidityName,
		Question: types.PerspectiveQuestion(LiquidityName),
		Classes: []types.PerspectiveClass{
			{State: "WallBuilding", Probability: 0.7},
			{State: "VacuumForming", Probability: 0.1},
			{State: "Replenishing", Probability: 0.1},
			{State: "Depleting", Probability: 0.05},
			{State: "Balanced", Probability: 0.05},
		},
		Lease: types.PerspectiveLease{
			Clock: liquidityClock,
			From:  1,
			Until: 2,
		},
		Round:     1,
		Lifecycle: types.PerspectiveIssued,
	}
}

func liquidityEnvelope(ordinal uint64, touchNotional float64) *types.Envelope {
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

	liqMeasurement := data.NewMeasurement[float64](
		"liquidity:BTC/USD",
		"BTC/USD",
		"liquidity",
		at,
		at,
	)
	liqMeasurement.PutMetric(data.NewMetric(
		"two_sided_touch_notional",
		touchNotional,
		nil,
		nil,
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
	))

	envelope := types.NewEnvelope(types.EnvelopeLevel3)
	envelope.Level3Data.Symbol = "BTC/USD"
	envelope.Level3Data.Timestamp = at
	envelope.PumpDump = pumpMeasurement
	envelope.Liquidity = liqMeasurement

	return envelope
}
