package advisor

import (
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestNewAuction(t *testing.T) {
	Convey("Given the declarative Auction Advisor", t, func() {
		auction := NewAuction()

		Convey("it exposes one Feature for each auction phase", func() {
			So(auction.Features, ShouldHaveLength, 5)
			So(auction.Features[0].Class.Label, ShouldEqual, "BuyersBreakingThrough")
			So(auction.Features[1].Class.Label, ShouldEqual, "SellersAbsorbing")
			So(auction.Features[2].Class.Label, ShouldEqual, "SellersBreakingThrough")
			So(auction.Features[3].Class.Label, ShouldEqual, "BuyersAbsorbing")
			So(auction.Features[4].Class.Label, ShouldEqual, "Balanced")
		})

		Convey("every selector is qualified and unique inside its Feature", func() {
			for _, feature := range auction.Features {
				So(feature.Clock, ShouldEqual, auctionClock)
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
				{"cvd/midpoint_response_per_net_notional", INCREASE, DECREASE},
				{"cvd/flow_aligned_midpoint_return", DISSOLVE, EXPAND},
				{"cvd/midpoint_response_per_net_notional", INCREASE, DECREASE},
				{"cvd/flow_aligned_midpoint_return", DISSOLVE, EXPAND},
				{"cvd/signed_net_fraction", STAGNATE, EXPAND},
			}

			for index, feature := range auction.Features {
				So(feature.Class.Within, ShouldEqual, auctionPredictionHorizon)
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

		Convey("the BuyersBreakingThrough observation survives the real Auction Arena", func() {
			node := &arenaNode{}
			arena, err := NewArena(AuctionName, auction.Features, node, 1)
			So(err, ShouldBeNil)
			issued := auctionEnvelope(1, 10.0)
			issued.Perspectives = []*types.Perspective{auctionPerspective()}
			So(arena.Step(issued), ShouldNotBeNil)

			supported := auctionEnvelope(2, 20.0)
			So(arena.Step(supported), ShouldEqual, supported)
			So(node.perspectives, ShouldHaveLength, 1)
			So(node.perspectives[0].Lifecycle, ShouldEqual, types.PerspectiveSurvived)
			So(node.perspectives[0].ResolvedBy, ShouldEqual,
				types.PerspectiveEvent("cvd/midpoint_response_per_net_notional"))
		})
	})
}

func auctionPerspective() *types.Perspective {
	return &types.Perspective{
		Symbol:   "BTC/USD",
		Advisor:  AuctionName,
		Question: types.PerspectiveQuestion(AuctionName),
		Classes: []types.PerspectiveClass{
			{State: "BuyersBreakingThrough", Probability: 0.7},
			{State: "SellersAbsorbing", Probability: 0.1},
			{State: "SellersBreakingThrough", Probability: 0.1},
			{State: "BuyersAbsorbing", Probability: 0.05},
			{State: "Balanced", Probability: 0.05},
		},
		Lease: types.PerspectiveLease{
			Clock: auctionClock,
			From:  1,
			Until: 2,
		},
		Round:     1,
		Lifecycle: types.PerspectiveIssued,
	}
}

func auctionEnvelope(ordinal uint64, response float64) *types.Envelope {
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

	cvdMeasurement := data.NewMeasurement[float64](
		"cvd:BTC/USD",
		"BTC/USD",
		"cvd",
		at,
		at,
	)
	cvdMeasurement.PutMetric(data.NewMetric(
		"midpoint_response_per_net_notional",
		response,
		nil,
		nil,
		data.UnitRate,
		data.TimescaleInstantaneous,
	))

	envelope := types.NewEnvelope(types.EnvelopeTrade)
	envelope.TradeData.Symbol = "BTC/USD"
	envelope.TradeData.Timestamp = at
	envelope.PumpDump = pumpMeasurement
	envelope.CVD = cvdMeasurement

	return envelope
}
