package broker

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func kinds(life Lifecycle) []string {
	out := make([]string, 0, len(life.Events))

	for _, event := range life.Events {
		out = append(out, event.Kind)
	}

	return out
}

func TestDesk_Lifecycle(t *testing.T) {
	Convey("Given a position entered on a learned match", t, func() {
		desk, depth, venue := deskFixture(200, 1000)
		edge := matched
		edge.Match = &Match{
			At: deskNow, Action: "enter", Path: "R01/R02/R03", Tokens: 3, Candidates: 2,
			Actions: map[string]int{"enter": 2}, Confidence: 3, Threshold: 3,
		}
		So(desk.Enter("BTC/USD", edge), ShouldBeNil)
		buy := venue.next()

		report := desk.Lifecycles()
		So(report.Lifecycles, ShouldHaveLength, 1)
		life := report.Lifecycles[0]

		Convey("The timeline opens with the match, the sizing inputs, the child order and its fill", func() {
			So(kinds(life)[:5], ShouldResemble, []string{
				EventEntryMatch, EventSizing, EventChildOrder, EventOrder, EventFill,
			})
			So(life.Events[0].Fields["path"], ShouldEqual, "R01/R02/R03")
			So(life.Events[0].Fields["threshold"], ShouldEqual, 3)
			So(life.Events[1].Fields["binding"], ShouldNotBeEmpty)
			So(life.Events[1].Fields, ShouldContainKey, "exit_capacity")
			So(life.Events[1].Fields, ShouldContainKey, "participation")
			So(life.Events[1].Fields, ShouldContainKey, "cash")
			So(life.Events[1].Fields, ShouldContainKey, "budget")
			So(life.Events[4].Fields["quantity"], ShouldEqual, buy.volume.String())
			So(life.Events[4].Fields["shadow_priced"], ShouldBeTrue)
			So(life.Status, ShouldEqual, "holding")
			So(report.Performance.Closed, ShouldEqual, 0)
			So(report.Performance.ShadowWinRate, ShouldBeNil)
		})

		Convey("A risk trim, the exit match, the learned exit and the outcome are appended in order", func() {
			persist(desk, depth, venue, thinBids, deskNow.Add(time.Second))
			trim := venue.next()
			So(trim.side, ShouldEqual, "sell")

			So(desk.Matched("BTC/USD", Match{At: deskNow.Add(time.Minute), Action: "exit", Path: "R04/R05"}), ShouldBeTrue)
			So(desk.Exit("BTC/USD"), ShouldBeNil)
			So(venue.next().side, ShouldEqual, "sell")

			life := desk.Lifecycles().Lifecycles[0]
			got := kinds(life)
			tail := got[5:]
			So(tail, ShouldResemble, []string{
				EventRiskSell, EventOrder, EventFill, EventExitMatch, EventExit, EventOrder, EventFill, EventClosed,
			})
			So(life.Events[5].Fields["trigger"], ShouldEqual, TriggerCapacityTrim)
			So(life.Events[5].Fields, ShouldContainKey, "capacity")
			So(life.Status, ShouldEqual, "closed")
			So(life.Outcome, ShouldNotBeNil)
			So(life.Outcome.Triggers, ShouldResemble, []string{TriggerCapacityTrim, TriggerLearnedExit})
			So(life.Outcome.ExpectedHold, ShouldEqual, time.Minute)

			perf := desk.Lifecycles().Performance
			So(perf.Closed, ShouldEqual, 1)
			So(perf.VenueWinRate, ShouldNotBeNil)
			So(perf.VenuePnl, ShouldAlmostEqual, life.Outcome.VenueRealized, 1e-12)
		})
	})
}

func TestPerformance(t *testing.T) {
	Convey("Performance measures win rate and mean return over closed outcomes only", t, func() {
		perf := performance([]Lifecycle{
			{Outcome: &Outcome{VenueCost: 100, VenueRealized: 2, ShadowDefined: true, ShadowCost: 100, ShadowRealized: 1}},
			{Outcome: &Outcome{VenueCost: 100, VenueRealized: -4, ShadowDefined: true, ShadowCost: 50, ShadowRealized: -5}},
			{Outcome: &Outcome{VenueCost: 100, VenueRealized: 1}},
			{Status: "holding"},
		})

		So(perf.Closed, ShouldEqual, 3)
		So(perf.ShadowTrades, ShouldEqual, 2)
		So(*perf.ShadowWinRate, ShouldAlmostEqual, 0.5, 1e-15)
		So(*perf.ShadowMeanReturn, ShouldAlmostEqual, (0.01-0.1)/2, 1e-15)
		So(perf.ShadowPnl, ShouldAlmostEqual, -4, 1e-15)
		So(*perf.VenueWinRate, ShouldAlmostEqual, 2.0/3, 1e-15)
		So(*perf.VenueMeanReturn, ShouldAlmostEqual, (0.02-0.04+0.01)/3, 1e-15)
	})
}

func TestLifecycleRecordNonFinite(t *testing.T) {
	Convey("A non-finite field is recorded by name so the report still marshals", t, func() {
		life := newLifecycle("BTC/USD")
		life.record(EventExit, "x", time.Time{}, map[string]any{"ratio": math.Inf(1), "open": 1.5})

		body, err := json.Marshal(life.copy())
		So(err, ShouldBeNil)
		So(string(body), ShouldContainSubstring, `"ratio":"+Inf"`)
		So(life.Events[0].Fields["open"], ShouldEqual, 1.5)
	})
}
