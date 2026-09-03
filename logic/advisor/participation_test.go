package advisor

import (
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestNewParticipation(t *testing.T) {
	Convey("Given the declarative Participation Advisor", t, func() {
		participation := NewParticipation()

		Convey("it exposes one Feature for each participation phase", func() {
			So(participation.Features, ShouldHaveLength, 4)
			So(participation.Features[0].Class.Label, ShouldEqual, "BroadLift")
			So(participation.Features[1].Class.Label, ShouldEqual, "LocalLeader")
			So(participation.Features[2].Class.Label, ShouldEqual, "FollowerMove")
			So(participation.Features[3].Class.Label, ShouldEqual, "IsolatedMove")
		})

		Convey("every selector is qualified and unique inside its Feature", func() {
			for _, feature := range participation.Features {
				So(feature.Clock, ShouldEqual, participationClock)
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
				{"sentiment/advance_fraction", INCREASE, DECREASE},
				{"correlation/relative_return_energy", INCREASE, DECREASE},
				{"leadlag/best_lag_seconds", DECREASE, INCREASE},
				{"correlation/relative_return_energy", EXPAND, DISSOLVE},
			}

			for index, feature := range participation.Features {
				So(feature.Class.Within, ShouldEqual, participationPredictionHorizon)
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

		Convey("the BroadLift observation survives the real Participation Arena", func() {
			node := &arenaNode{}
			arena, err := NewArena(ParticipationName, participation.Features, node, 1)
			So(err, ShouldBeNil)
			issued := participationEnvelope(1, 0.6)
			issued.Perspectives = []*types.Perspective{participationPerspective()}
			So(arena.Step(issued), ShouldNotBeNil)

			supported := participationEnvelope(2, 0.8)
			So(arena.Step(supported), ShouldEqual, supported)
			So(node.perspectives, ShouldHaveLength, 1)
			So(node.perspectives[0].Lifecycle, ShouldEqual, types.PerspectiveSurvived)
			So(node.perspectives[0].ResolvedBy, ShouldEqual,
				types.PerspectiveEvent("sentiment/advance_fraction"))
		})
	})
}

func participationPerspective() *types.Perspective {
	return &types.Perspective{
		Symbol:   "BTC/USD",
		Advisor:  ParticipationName,
		Question: types.PerspectiveQuestion(ParticipationName),
		Classes: []types.PerspectiveClass{
			{State: "BroadLift", Probability: 0.7},
			{State: "LocalLeader", Probability: 0.1},
			{State: "FollowerMove", Probability: 0.1},
			{State: "IsolatedMove", Probability: 0.1},
		},
		Lease: types.PerspectiveLease{
			Clock: participationClock,
			From:  1,
			Until: 2,
		},
		Round:     1,
		Lifecycle: types.PerspectiveIssued,
	}
}

func participationEnvelope(ordinal uint64, advanceFraction float64) *types.Envelope {
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

	sentimentMeasurement := data.NewMeasurement[float64](
		"sentiment:BTC/USD",
		"BTC/USD",
		"sentiment",
		at,
		at,
	)
	sentimentMeasurement.PutMetric(data.NewMetric(
		"advance_fraction",
		advanceFraction,
		nil,
		nil,
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
	))

	envelope := types.NewEnvelope(types.EnvelopeTrade)
	envelope.TradeData.Symbol = "BTC/USD"
	envelope.TradeData.Timestamp = at
	envelope.PumpDump = pumpMeasurement
	envelope.Sentiment = sentimentMeasurement

	return envelope
}
