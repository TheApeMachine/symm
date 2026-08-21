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
			// One second before the leg's buy, the system looked at the symbol
			// and declined to act; hindsight pins that moment's scores to the
			// missed leg.
			decisionAt(-1, "nothing", map[string]float64{
				"meas:TEST/USD:depthflow:neutral_score": 0.05,
			}),
		}
		decisions[0].Reason = "planner: structural thesis below admission"
		decisions[0].ThesisConfidence = 0.4
		decisions[0].AdmissionThreshold = 0.5

		reports, err := Analyze(reducer, decisions)
		So(err, ShouldBeNil)

		Convey("It should report the full rise as missed, with signal context", func() {
			So(len(reports), ShouldEqual, 1)
			report := reports[0]
			So(report.Symbol, ShouldEqual, "TEST/USD")
			So(report.UpboundPct, ShouldAlmostEqual, 0.2, 1e-9)
			So(report.MissedPct, ShouldAlmostEqual, 0.2, 1e-9)
			So(report.RealizedPct, ShouldAlmostEqual, 0.0, 1e-9)
			So(report.Legs, ShouldEqual, 1)
			So(report.MissedLegs, ShouldEqual, 1)

			Convey("The missed leg should carry the last decision's scores", func() {
				So(len(report.Opportunities), ShouldEqual, 1)
				opp := report.Opportunities[0]
				So(opp.Missed, ShouldBeTrue)
				So(opp.Leg.BuyPrice, ShouldAlmostEqual, 100, 1e-9)
				So(opp.Leg.SellPrice, ShouldAlmostEqual, 120, 1e-9)
				So(opp.Signal.Action, ShouldEqual, "nothing")
				So(opp.Signal.Reason, ShouldEqual, "planner: structural thesis below admission")
				So(opp.Signal.ThesisConfidence, ShouldAlmostEqual, 0.4, 1e-9)
				So(opp.Signal.AdmissionThreshold, ShouldAlmostEqual, 0.5, 1e-9)
				So(opp.Signal.Alternatives["meas:TEST/USD:depthflow:neutral_score"], ShouldAlmostEqual, 0.05, 1e-9)
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

		reports, err := Analyze(reducer, decisions)
		So(err, ShouldBeNil)

		Convey("It should mark the leg captured, not missed", func() {
			So(len(reports), ShouldEqual, 1)
			report := reports[0]
			So(report.MissedLegs, ShouldEqual, 0)
			So(report.MissedPct, ShouldAlmostEqual, 0.0, 1e-9)
			So(report.UpboundPct, ShouldAlmostEqual, 0.2, 1e-9)
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
			ThesisScore:     0.6,
			Alternatives:    map[string]float64{},
		}

		reports, err := Analyze(reducer, []Decision{opportunity})
		So(err, ShouldBeNil)

		Convey("It should carry the opportunity classification onto the missed leg", func() {
			So(len(reports), ShouldEqual, 1)
			So(len(reports[0].Opportunities), ShouldEqual, 1)
			opp := reports[0].Opportunities[0]
			So(opp.Signal.Opportunity, ShouldBeTrue)
			So(opp.Signal.Type, ShouldEqual, "pump")
			So(opp.Signal.ThesisScore, ShouldAlmostEqual, 0.6, 1e-9)
		})
	})

	Convey("Given a decision before the leg buy but an enter after the leg ends", t, func() {
		reducer := NewReducer()
		payload := []byte(`{"channel":"trade","type":"update","data":[
			{"symbol":"TEST/USD","side":"buy","price":100,"qty":1,"timestamp":"1970-01-01T00:00:00Z"},
			{"symbol":"TEST/USD","side":"buy","price":120,"qty":1,"timestamp":"1970-01-01T00:00:02Z"}
		]}`)
		So(reducer.Ingest(payload, epoch), ShouldBeNil)

		// Enter only after the sell; the leg window (0s..2s) has no enter.
		decisions := []Decision{
			decisionAt(3, "enter", map[string]float64{}),
		}

		reports, err := Analyze(reducer, decisions)
		So(err, ShouldBeNil)

		Convey("It should still count the leg as missed", func() {
			So(reports[0].MissedLegs, ShouldEqual, 1)
			So(reports[0].MissedPct, ShouldAlmostEqual, 0.2, 1e-9)
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

		reports, err := Analyze(reducer, []Decision{})
		So(err, ShouldBeNil)

		Convey("It should return reports sorted by missed profit, most valuable first", func() {
			So(len(reports), ShouldEqual, 2)
			So(reports[0].Symbol, ShouldEqual, "BBB/USD")
			So(reports[1].Symbol, ShouldEqual, "AAA/USD")
		})
	})
}
