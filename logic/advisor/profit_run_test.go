package advisor

import (
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func TestNewProfitRun(t *testing.T) {
	Convey("Given the declarative ProfitRun Advisor", t, func() {
		profitRun := NewProfitRun()

		Convey("it exposes one Feature for each profit-run phase", func() {
			So(profitRun.Features, ShouldHaveLength, 4)
			So(profitRun.Features[0].Class.Label, ShouldEqual, "Extending")
			So(profitRun.Features[1].Class.Label, ShouldEqual, "Consolidating")
			So(profitRun.Features[2].Class.Label, ShouldEqual, "Exhausting")
			So(profitRun.Features[3].Class.Label, ShouldEqual, "GivingBack")
		})

		Convey("every selector is qualified and unique inside its Feature", func() {
			for _, feature := range profitRun.Features {
				So(feature.Clock, ShouldEqual, profitRunClock)
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
				{"pumpdump/midpoint_return_velocity", INCREASE, DECREASE},
				{"pumpdump/midpoint_log_return", STAGNATE, EXPAND},
				{"cvd/flow_aligned_midpoint_return", DISSOLVE, EXPAND},
				{"pumpdump/midpoint_return_velocity", DECREASE, INCREASE},
			}

			for index, feature := range profitRun.Features {
				So(feature.Class.Within, ShouldEqual, profitRunPredictionHorizon)
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

		Convey("the Extending observation survives the real ProfitRun Arena", func() {
			node := &arenaNode{}
			arena, err := NewArena(ProfitRunName, profitRun.Features, node, 1)
			So(err, ShouldBeNil)
			issued := profitRunEnvelope(1, 0.001)
			issued.Perspectives = []*types.Perspective{profitRunPerspective()}
			So(arena.Step(issued), ShouldNotBeNil)

			supported := profitRunEnvelope(2, 0.005)
			So(arena.Step(supported), ShouldEqual, supported)
			So(node.perspectives, ShouldHaveLength, 1)
			So(node.perspectives[0].Lifecycle, ShouldEqual, types.PerspectiveSurvived)
			So(node.perspectives[0].ResolvedBy, ShouldEqual,
				types.PerspectiveEvent("pumpdump/midpoint_return_velocity"))
		})
	})
}

func profitRunPerspective() *types.Perspective {
	return &types.Perspective{
		Symbol:   "BTC/USD",
		Advisor:  nmtypes.MustIntern(ProfitRunName),
		Question: types.PerspectiveQuestion(ProfitRunName),
		Classes: []types.PerspectiveClass{
			{State: "Extending", Probability: 0.7},
			{State: "Consolidating", Probability: 0.1},
			{State: "Exhausting", Probability: 0.1},
			{State: "GivingBack", Probability: 0.1},
		},
		Lease: types.PerspectiveLease{
			Clock: nmtypes.MustIntern(profitRunClock),
			From:  1,
			Until: 2,
		},
		Round:     1,
		Lifecycle: types.PerspectiveIssued,
	}
}

func profitRunEnvelope(ordinal uint64, velocity float64) *types.Envelope {
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
	pumpMeasurement.PutMetric(data.NewMetric(
		"midpoint_return_velocity",
		velocity,
		nil,
		nil,
		data.UnitPerSecond,
		data.TimescalePerSecond,
	))

	envelope := types.NewEnvelope(types.EnvelopeTrade)
	envelope.TradeData.Symbol = "BTC/USD"
	envelope.TradeData.Timestamp = at
	envelope.PumpDump = pumpMeasurement

	return envelope
}
