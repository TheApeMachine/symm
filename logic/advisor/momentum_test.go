package advisor

import (
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestNewMomentum(t *testing.T) {
	Convey("Given the declarative Momentum Advisor", t, func() {
		momentum := NewMomentum()

		Convey("it exposes one Feature for each momentum phase", func() {
			So(momentum.Features, ShouldHaveLength, 4)
			So(momentum.Features[0].Class.Label, ShouldEqual, "Building")
			So(momentum.Features[1].Class.Label, ShouldEqual, "Sustaining")
			So(momentum.Features[2].Class.Label, ShouldEqual, "Stalling")
			So(momentum.Features[3].Class.Label, ShouldEqual, "Reversing")
		})

		Convey("every selector is qualified and unique inside its Feature", func() {
			for _, feature := range momentum.Features {
				So(feature.Clock, ShouldEqual, momentumClock)
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
				{"pumpdump/notional_rate_velocity", INCREASE, DECREASE},
				{"cvd/flow_aligned_midpoint_return", EXPAND, DISSOLVE},
				{"cvd/flow_aligned_midpoint_return", DISSOLVE, EXPAND},
				{"pumpdump/midpoint_return_divergence", EXPAND, DISSOLVE},
			}

			for index, feature := range momentum.Features {
				So(feature.Class.Within, ShouldEqual, momentumPredictionHorizon)
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

		Convey("the Building observation survives the real Momentum Arena", func() {
			node := &arenaNode{}
			arena, err := NewArena(MomentumName, momentum.Features, node, 1)
			So(err, ShouldBeNil)
			issued := momentumEnvelope(1, 2)
			issued.Perspectives = []*types.Perspective{momentumPerspective()}
			So(arena.Step(issued), ShouldNotBeNil)

			supported := momentumEnvelope(2, 3)
			So(arena.Step(supported), ShouldEqual, supported)
			So(node.perspectives, ShouldHaveLength, 1)
			So(node.perspectives[0].Lifecycle, ShouldEqual, types.PerspectiveSurvived)
			So(node.perspectives[0].ResolvedBy, ShouldEqual,
				types.PerspectiveEvent("pumpdump/notional_rate_velocity"))
		})
	})
}

func momentumPerspective() *types.Perspective {
	return &types.Perspective{
		Symbol:   "BTC/USD",
		Advisor:  MomentumName,
		Question: types.PerspectiveQuestion(MomentumName),
		Classes: []types.PerspectiveClass{
			{State: "Building", Probability: 0.7},
			{State: "Sustaining", Probability: 0.1},
			{State: "Stalling", Probability: 0.1},
			{State: "Reversing", Probability: 0.1},
		},
		Lease: types.PerspectiveLease{
			Clock: momentumClock,
			From:  1,
			Until: 2,
		},
		Round:     1,
		Lifecycle: types.PerspectiveIssued,
	}
}

func momentumEnvelope(ordinal uint64, velocity float64) *types.Envelope {
	at := time.Unix(1_700_000_000+int64(ordinal), 0)
	measurement := data.NewMeasurement[float64](
		"pumpdump:BTC/USD",
		"BTC/USD",
		"pumpdump",
		at,
		at,
	)
	measurement.PutMetric(data.NewMetric(
		"completed_volume_bar_ordinal",
		float64(ordinal),
		nil,
		nil,
		data.UnitCount,
		data.TimescaleInstantaneous,
	))
	measurement.PutMetric(data.NewMetric(
		"notional_rate_velocity",
		velocity,
		nil,
		nil,
		data.UnitPerSecond,
		data.TimescalePerSecond,
	))

	envelope := types.NewEnvelope(types.EnvelopeTrade)
	envelope.TradeData.Symbol = "BTC/USD"
	envelope.TradeData.Timestamp = at
	envelope.PumpDump = measurement

	return envelope
}
