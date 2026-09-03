package advisor

import (
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestNewPullback(t *testing.T) {
	Convey("Given the declarative Pullback Advisor", t, func() {
		pullback := NewPullback()

		Convey("it exposes one Feature for each pullback phase", func() {
			So(pullback.Features, ShouldHaveLength, 4)
			So(pullback.Features[0].Class.Label, ShouldEqual, "OrderlyPullback")
			So(pullback.Features[1].Class.Label, ShouldEqual, "LiquiditySweep")
			So(pullback.Features[2].Class.Label, ShouldEqual, "StructuralBreakdown")
			So(pullback.Features[3].Class.Label, ShouldEqual, "Unresolved")
		})

		Convey("every selector is qualified and unique inside its Feature", func() {
			for _, feature := range pullback.Features {
				So(feature.Clock, ShouldEqual, pullbackClock)
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
				{"cvd/gross_notional_rate", DECREASE, INCREASE},
				{"toxicity/net_replenishment_fraction:bid", INCREASE, DECREASE},
				{"toxicity/net_withdrawal_fraction:bid", EXPAND, DISSOLVE},
				{"hawkes/branching_spectral_radius", STAGNATE, EXPAND},
			}

			for index, feature := range pullback.Features {
				So(feature.Class.Within, ShouldEqual, pullbackPredictionHorizon)
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

		Convey("the LiquiditySweep observation survives the real Pullback Arena", func() {
			node := &arenaNode{}
			arena, err := NewArena(PullbackName, pullback.Features, node, 1)
			So(err, ShouldBeNil)
			issued := pullbackEnvelope(1, 0.2)
			issued.Perspectives = []*types.Perspective{pullbackPerspective()}
			So(arena.Step(issued), ShouldNotBeNil)

			supported := pullbackEnvelope(2, 0.6)
			So(arena.Step(supported), ShouldEqual, supported)
			So(node.perspectives, ShouldHaveLength, 1)
			So(node.perspectives[0].Lifecycle, ShouldEqual, types.PerspectiveSurvived)
			So(node.perspectives[0].ResolvedBy, ShouldEqual,
				types.PerspectiveEvent("toxicity/net_replenishment_fraction:bid"))
		})
	})
}

func pullbackPerspective() *types.Perspective {
	return &types.Perspective{
		Symbol:   "BTC/USD",
		Advisor:  PullbackName,
		Question: types.PerspectiveQuestion(PullbackName),
		Classes: []types.PerspectiveClass{
			{State: "OrderlyPullback", Probability: 0.1},
			{State: "LiquiditySweep", Probability: 0.7},
			{State: "StructuralBreakdown", Probability: 0.1},
			{State: "Unresolved", Probability: 0.1},
		},
		Lease: types.PerspectiveLease{
			Clock: pullbackClock,
			From:  1,
			Until: 2,
		},
		Round:     1,
		Lifecycle: types.PerspectiveIssued,
	}
}

func pullbackEnvelope(ordinal uint64, replenishment float64) *types.Envelope {
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

	toxicityMeasurement := data.NewMeasurement[float64](
		"toxicity:BTC/USD",
		"BTC/USD",
		"toxicity",
		at,
		at,
	)
	toxicityMeasurement.PutMetric(data.NewMetric(
		"net_replenishment_fraction:bid",
		replenishment,
		nil,
		nil,
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
	))

	envelope := types.NewEnvelope(types.EnvelopeTrade)
	envelope.TradeData.Symbol = "BTC/USD"
	envelope.TradeData.Timestamp = at
	envelope.PumpDump = pumpMeasurement
	envelope.Toxicity = toxicityMeasurement

	return envelope
}
