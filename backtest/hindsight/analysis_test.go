package hindsight

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
decisionAt builds a Decision at a given second offset from epoch with the
given action and alternatives.
*/
func decisionAt(second int, action string, alternatives map[string]float64) Decision {
	return Decision{
		Action:       action,
		Symbol:       "TEST/USD",
		At:           epoch.Add(time.Duration(second) * time.Second),
		Alternatives: alternatives,
	}
}

func TestAnalyze(t *testing.T) {
	Convey("Given a rising tape and only nothing decisions", t, func() {
		reducer := NewReducer()
		payload := []byte(`{"channel":"trade","type":"update","data":[
			{"symbol":"TEST/USD","side":"buy","price":100,"qty":1,"timestamp":"1970-01-01T00:00:00Z"},
			{"symbol":"TEST/USD","side":"buy","price":120,"qty":1,"timestamp":"1970-01-01T00:00:02Z"}
		]}`)
		So(reducer.Ingest(payload, epoch), ShouldBeNil)

		decisions := []Decision{
			decisionAt(-1, "nothing", map[string]float64{
				"meas:TEST/USD:depthflow:neutral_score": 0.05,
			}),
		}
		decisions[0].Reason = "planner: economic consequence not estimable"
		decisions[0].ValuationAttempted = true
		decisions[0].ValuationAvailable = false
		decisions[0].ValuationStatus = "incomplete"

		reports, err := Analyze(reducer, decisions, nil, time.Time{})
		So(err, ShouldBeNil)

		Convey("It should report the full rise as missed, with valuation context", func() {
			So(len(reports), ShouldEqual, 1)
			report := reports[0]
			So(report.Symbol, ShouldEqual, "TEST/USD")
			So(report.PriceTheoreticalCeiling, ShouldAlmostEqual, 0.2, 1e-9)
			So(report.MissedPct, ShouldAlmostEqual, 0.2, 1e-9)
			So(report.RealizedPct, ShouldAlmostEqual, 0.0, 1e-9)
			So(report.Legs, ShouldEqual, 1)
			So(report.MissedLegs, ShouldEqual, 1)

			Convey("The missed leg should carry the valuation state", func() {
				So(len(report.Opportunities), ShouldEqual, 1)
				opp := report.Opportunities[0]
				So(opp.Missed, ShouldBeTrue)
				So(opp.Leg.BuyPrice, ShouldAlmostEqual, 100, 1e-9)
				So(opp.Leg.SellPrice, ShouldAlmostEqual, 120, 1e-9)
				So(opp.Signal.Action, ShouldEqual, "nothing")
				So(opp.Signal.Reason, ShouldEqual, "planner: economic consequence not estimable")
				So(opp.Signal.ValuationAttempted, ShouldBeTrue)
				So(opp.Signal.ValuationAvailable, ShouldBeFalse)
				So(opp.Signal.Alternatives["meas:TEST/USD:depthflow:neutral_score"], ShouldAlmostEqual, 0.05, 1e-9)
			})

			Convey("Valuation regret should be owned by the valuation layer", func() {
				opp := report.Opportunities[0]
				So(opp.Regret.Valuation, ShouldBeTrue)
			})
		})
	})

	Convey("Given a rising tape with an enter inside the leg window", t, func() {
		reducer := NewReducer()
		payload := []byte(`{"channel":"trade","type":"update","data":[
			{"symbol":"TEST/USD","side":"buy","price":100,"qty":1,"timestamp":"1970-01-01T00:00:00Z"},
			{"symbol":"TEST/USD","side":"buy","price":120,"qty":1,"timestamp":"1970-01-01T00:00:02Z"}
		]}`)
		So(reducer.Ingest(payload, epoch), ShouldBeNil)

		decisions := []Decision{
			decisionAt(1, "enter", map[string]float64{}),
		}

		reports, err := Analyze(reducer, decisions, nil, time.Time{})
		So(err, ShouldBeNil)

		Convey("It should mark the leg captured, not missed", func() {
			So(len(reports), ShouldEqual, 1)
			report := reports[0]
			So(report.MissedLegs, ShouldEqual, 0)
			So(report.MissedPct, ShouldAlmostEqual, 0.0, 1e-9)
			So(report.PriceTheoreticalCeiling, ShouldAlmostEqual, 0.2, 1e-9)
			So(report.RealizedPct, ShouldAlmostEqual, 0.2, 1e-9)
			So(len(report.Opportunities), ShouldEqual, 0)
		})
	})

	Convey("Given a missed leg flagged as a real opportunity by the logic", t, func() {
		reducer := NewReducer()
		payload := []byte(`{"channel":"trade","type":"update","data":[
			{"symbol":"TEST/USD","side":"buy","price":100,"qty":1,"timestamp":"1970-01-01T00:00:00Z"},
			{"symbol":"TEST/USD","side":"buy","price":150,"qty":1,"timestamp":"1970-01-01T00:00:02Z"}
		]}`)
		So(reducer.Ingest(payload, epoch), ShouldBeNil)

		opportunity := Decision{
			Action:          "nothing",
			Symbol:          "TEST/USD",
			At:              epoch.Add(-time.Second),
			Opportunity:     true,
			OpportunityType: "pump",
			Alternatives:    map[string]float64{},
		}

		reports, err := Analyze(reducer, []Decision{opportunity}, nil, time.Time{})
		So(err, ShouldBeNil)

		Convey("It should carry the opportunity classification onto the missed leg", func() {
			So(len(reports), ShouldEqual, 1)
			So(len(reports[0].Opportunities), ShouldEqual, 1)
			opp := reports[0].Opportunities[0]
			So(opp.Signal.Opportunity, ShouldBeTrue)
			So(opp.Signal.OpportunityType, ShouldEqual, "pump")
		})
	})

	Convey("Given a move that began before the observer started", t, func() {
		reducer := NewReducer()
		payload := []byte(`{"channel":"trade","type":"update","data":[
			{"symbol":"TEST/USD","side":"buy","price":100,"qty":1,"timestamp":"1970-01-01T00:00:00Z"},
			{"symbol":"TEST/USD","side":"buy","price":120,"qty":1,"timestamp":"1970-01-01T00:00:02Z"}
		]}`)
		So(reducer.Ingest(payload, epoch), ShouldBeNil)

		// Observer came online at t=1s, halfway through the move.
		observerStarted := epoch.Add(time.Second)

		reports, err := Analyze(reducer, []Decision{}, nil, observerStarted)
		So(err, ShouldBeNil)

		Convey("The pre-observer portion must not count as missed strategy value", func() {
			So(len(reports), ShouldEqual, 1)
			// Only the post-observer portion (1s→2s) is leg-time; but the leg
			// spans 0s→2s, so its buy pre-dates observation and it is excluded
			// from missed legs entirely.
			So(reports[0].MissedLegs, ShouldEqual, 0)
			So(reports[0].MissedPct, ShouldAlmostEqual, 0.0, 1e-9)
		})
	})

	Convey("Given two symbols where a strict earlier enter shifts the ordering", t, func() {
		reducer := NewReducer()
		public := []byte(`{"channel":"trade","type":"update","data":[
			{"symbol":"AAA/USD","side":"buy","price":100,"qty":1,"timestamp":"1970-01-01T00:00:00Z"},
			{"symbol":"AAA/USD","side":"buy","price":101,"qty":1,"timestamp":"1970-01-01T00:00:01Z"},
			{"symbol":"BBB/USD","side":"buy","price":100,"qty":1,"timestamp":"1970-01-01T00:00:00Z"},
			{"symbol":"BBB/USD","side":"buy","price":122,"qty":1,"timestamp":"1970-01-01T00:00:01Z"}
		]}`)
		So(reducer.Ingest(public, epoch), ShouldBeNil)

		reports, err := Analyze(reducer, []Decision{}, nil, time.Time{})
		So(err, ShouldBeNil)

		Convey("It should return reports sorted by missed profit, most valuable first", func() {
			So(len(reports), ShouldEqual, 2)
			So(reports[0].Symbol, ShouldEqual, "BBB/USD")
			So(reports[1].Symbol, ShouldEqual, "AAA/USD")
		})
	})
}

func TestDecisionsAround(t *testing.T) {
	leg := Leg{
		BuyAt:  epoch.Add(2 * time.Second),
		SellAt: epoch.Add(4 * time.Second),
	}

	decisions := []Decision{
		decisionAt(0, "nothing", nil),
		decisionAt(1, "nothing", nil),
		decisionAt(3, "nothing", nil),
		decisionAt(5, "nothing", nil),
		decisionAt(9, "nothing", nil),
	}

	Convey("Given decisions spanning a leg's window", t, func() {
		journal := decisionsAround(decisions, leg)

		Convey("It should span just before entry through just after exit", func() {
			So(len(journal), ShouldEqual, 3)
			So(journal[0].At, ShouldEqual, decisions[1].At)
			So(journal[len(journal)-1].At, ShouldEqual, decisions[3].At)
		})
	})
}
