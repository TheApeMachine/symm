package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestPostMortemEvaluate(t *testing.T) {
	Convey("Given a reconciled round trip with its post-exit tail complete", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Decisions = append(thesis.Decisions,
			types.Decision{
				Action: "enter", Symbol: "BTC/USD", Utility: 0.01,
				ExpectedReturn: 0.02, Uncertainty: 0.005,
				ForecastSource: "manifold_forecast", ForecastModel: "rls-next-l3-v1",
				ForecastEpoch: 10, Reason: "executable return exceeded doing nothing",
			},
			types.Decision{
				Action: "exit", Symbol: "BTC/USD",
				Reason: "exit utility exceeded continuation",
			},
		)
		thesis.TradeJournal = append(thesis.TradeJournal,
			types.TradeObservation{
				Kind: "broker_acceptance", Action: "enter", Symbol: "BTC/USD",
				At: time.Unix(1, 0),
			},
			types.TradeObservation{
				Kind: "execution", Symbol: "BTC/USD", Side: "buy",
				ExecutionID: "buy-fill", At: time.Unix(2, 0),
			},
			types.TradeObservation{
				Kind: "execution", Symbol: "BTC/USD", Side: "sell",
				ExecutionID: "sell-fill", At: time.Unix(3, 0),
			},
			types.TradeObservation{
				Kind: "final_outcome", Symbol: "BTC/USD", Fee: "2", PnL: "8",
				ReturnPct: 7.92, At: time.Unix(3, 0),
			},
			types.TradeObservation{
				Kind: "position_snapshot", Symbol: "BTC/USD", Status: "closed",
				Quantity: "0.00000000", At: time.Unix(3, 0),
			},
		)
		thesis.Lifecycle["BTC/USD"] = types.LifecyclePostMortemReady

		err := (&PostMortem{}).Evaluate(thesis, "BTC/USD")

		Convey("Then separate forecast, decision, and execution findings are retained", func() {
			So(err, ShouldBeNil)
			So(thesis.LifecycleState("BTC/USD"), ShouldEqual, types.LifecycleEvaluated)
			So(thesis.Findings, ShouldHaveLength, 3)
			So(thesis.Findings[0].Symbol, ShouldEqual, "BTC/USD")
			So(thesis.Findings[0].Component, ShouldEqual, "forecast")
			So(thesis.Findings[1].Component, ShouldEqual, "decision")
			So(thesis.Findings[2].Component, ShouldEqual, "execution")
			So(thesis.Findings[2].EstimatedEffect, ShouldEqual, 7.92)
		})
	})

	Convey("Given a position reconciled from exchange history before strategy observation", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action: "exit", Symbol: "BTC/USD",
			Reason: "exit utility exceeded continuation",
		})
		thesis.TradeJournal = append(thesis.TradeJournal,
			types.TradeObservation{
				Kind: "position_reconciliation", Symbol: "BTC/USD", Side: "buy",
				ExecutionID: "historical-buy", At: time.Unix(1, 0),
			},
			types.TradeObservation{
				Kind: "execution", Symbol: "BTC/USD", Side: "sell",
				ExecutionID: "sell-fill", At: time.Unix(2, 0),
			},
			types.TradeObservation{
				Kind: "final_outcome", Symbol: "BTC/USD", Fee: "2", PnL: "8",
				ReturnPct: 7.92, At: time.Unix(2, 0),
			},
			types.TradeObservation{
				Kind: "position_snapshot", Symbol: "BTC/USD", Status: "closed",
				Quantity: "0.00000000", At: time.Unix(2, 0),
			},
		)
		thesis.Lifecycle["BTC/USD"] = types.LifecyclePostMortemReady

		err := (&PostMortem{}).Evaluate(thesis, "BTC/USD")

		Convey("Then evaluation records the known boundary without inventing an entry decision", func() {
			So(err, ShouldBeNil)
			So(thesis.LifecycleState("BTC/USD"), ShouldEqual, types.LifecycleEvaluated)
			So(thesis.Findings, ShouldHaveLength, 1)
			So(thesis.Findings[0].Symbol, ShouldEqual, "BTC/USD")
			So(thesis.Findings[0].Component, ShouldEqual, "reconciliation")
			So(thesis.Findings[0].Evidence, ShouldResemble,
				[]string{"historical-buy", "sell-fill"})
		})
	})

	Convey("Given a journal whose stale events appear after the latest facts", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Decisions = append(thesis.Decisions,
			types.Decision{
				Action: "enter", Symbol: "BTC/USD", Utility: 0.01,
				ExpectedReturn: 0.02, Uncertainty: 0.005,
				ForecastSource: "manifold_forecast", ForecastModel: "rls-next-l3-v1",
				ForecastEpoch: 10, Reason: "executable return exceeded doing nothing",
			},
			types.Decision{
				Action: "exit", Symbol: "BTC/USD",
				Reason: "exit utility exceeded continuation",
			},
		)
		thesis.TradeJournal = append(thesis.TradeJournal,
			types.TradeObservation{
				Kind: "final_outcome", Symbol: "BTC/USD", Fee: "2", PnL: "8",
				ReturnPct: 7.92, At: time.Unix(4, 0),
			},
			types.TradeObservation{
				Kind: "position_snapshot", Symbol: "BTC/USD", Status: "closed",
				Quantity: "0.00000000", At: time.Unix(4, 0),
			},
			types.TradeObservation{
				Kind: "execution", Symbol: "BTC/USD", Side: "sell",
				ExecutionID: "sell-fill", At: time.Unix(3, 0),
			},
			types.TradeObservation{
				Kind: "execution", Symbol: "BTC/USD", Side: "buy",
				ExecutionID: "buy-fill", At: time.Unix(2, 0),
			},
			types.TradeObservation{
				Kind: "broker_acceptance", Action: "enter", Symbol: "BTC/USD",
				At: time.Unix(1, 0),
			},
			types.TradeObservation{
				Kind: "final_outcome", Symbol: "BTC/USD", Fee: "", PnL: "",
				At: time.Unix(2, 0),
			},
			types.TradeObservation{
				Kind: "position_snapshot", Symbol: "BTC/USD", Status: "closed",
				Quantity: "1.00000000", At: time.Unix(2, 0),
			},
		)
		thesis.Lifecycle["BTC/USD"] = types.LifecyclePostMortemReady

		err := (&PostMortem{}).Evaluate(thesis, "BTC/USD")

		Convey("Then evaluation uses the latest timestamp for each stage", func() {
			So(err, ShouldBeNil)
			So(thesis.LifecycleState("BTC/USD"), ShouldEqual, types.LifecycleEvaluated)
		})
	})

	Convey("Given a journal whose exit precedes its entry fill", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Decisions = append(thesis.Decisions,
			types.Decision{Action: "enter", Symbol: "BTC/USD"},
			types.Decision{Action: "exit", Symbol: "BTC/USD"},
		)
		thesis.TradeJournal = append(thesis.TradeJournal,
			types.TradeObservation{
				Kind: "broker_acceptance", Action: "enter", Symbol: "BTC/USD",
				At: time.Unix(1, 0),
			},
			types.TradeObservation{
				Kind: "execution", Symbol: "BTC/USD", Side: "sell",
				ExecutionID: "sell-fill", At: time.Unix(2, 0),
			},
			types.TradeObservation{
				Kind: "execution", Symbol: "BTC/USD", Side: "buy",
				ExecutionID: "buy-fill", At: time.Unix(3, 0),
			},
			types.TradeObservation{
				Kind: "final_outcome", Symbol: "BTC/USD", Fee: "2", PnL: "8",
				At: time.Unix(4, 0),
			},
			types.TradeObservation{
				Kind: "position_snapshot", Symbol: "BTC/USD", Status: "closed",
				Quantity: "0.00000000", At: time.Unix(4, 0),
			},
		)
		thesis.Lifecycle["BTC/USD"] = types.LifecyclePostMortemReady

		err := (&PostMortem{}).Evaluate(thesis, "BTC/USD")

		Convey("Then the out-of-order journal is rejected", func() {
			So(err, ShouldNotBeNil)
			So(thesis.LifecycleState("BTC/USD"), ShouldEqual, types.LifecycleInvalid)
		})
	})
}

func BenchmarkPostMortemEvaluate(b *testing.B) {
	postMortem := &PostMortem{}

	b.ReportAllocs()

	for b.Loop() {
		thesis := types.NewThesis(nil)
		thesis.Decisions = append(thesis.Decisions,
			types.Decision{
				Action: "enter", Symbol: "BTC/USD", ForecastSource: "manifold_forecast",
				ForecastModel: "rls-next-l3-v1", ForecastEpoch: 10,
			},
			types.Decision{Action: "exit", Symbol: "BTC/USD"},
		)
		thesis.TradeJournal = append(thesis.TradeJournal,
			types.TradeObservation{
				Kind: "broker_acceptance", Action: "enter", Symbol: "BTC/USD",
				At: time.Unix(1, 0),
			},
			types.TradeObservation{
				Kind: "execution", Symbol: "BTC/USD", Side: "buy",
				ExecutionID: "buy-fill", At: time.Unix(2, 0),
			},
			types.TradeObservation{
				Kind: "execution", Symbol: "BTC/USD", Side: "sell",
				ExecutionID: "sell-fill", At: time.Unix(3, 0),
			},
			types.TradeObservation{
				Kind: "final_outcome", Symbol: "BTC/USD", Fee: "2", PnL: "8",
				At: time.Unix(3, 0),
			},
			types.TradeObservation{
				Kind: "position_snapshot", Symbol: "BTC/USD", Status: "closed",
				Quantity: "0", At: time.Unix(3, 0),
			},
		)
		thesis.Lifecycle["BTC/USD"] = types.LifecyclePostMortemReady

		if err := postMortem.Evaluate(thesis, "BTC/USD"); err != nil {
			b.Fatal(err)
		}
	}
}
