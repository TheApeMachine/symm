package hindsight

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExtractLosses(t *testing.T) {
	Convey("Given a session with entered trades", t, func() {
		entryTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		exitTime := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)

		series := &Series{
			Symbol: "SOL/USD",
			Points: []Point{
				{At: entryTime, Price: 100, Bid: 99.9, Ask: 100.1, Friction: 0.002},
				{At: entryTime.Add(30 * time.Second), Price: 95, Bid: 94.9, Ask: 95.1, Friction: 0.002},
				{At: exitTime, Price: 90, Bid: 89.9, Ask: 90.1, Friction: 0.002},
			},
		}

		decisions := []Decision{
			{
				ID:                  "dec-entry-1",
				Symbol:              "SOL/USD",
				At:                  entryTime,
				Action:              "enter",
				Reason:              "pump continuation signal",
				ThesisScore:         0.75,
				ThesisSupport:       0.8,
				ThesisContradiction: 0.1,
			},
			{
				ID:                  "dec-hold-1",
				Symbol:              "SOL/USD",
				At:                  entryTime.Add(30 * time.Second),
				Action:              "wait",
				Reason:              "holding position",
				ThesisScore:         0.5,
				ThesisSupport:       0.4,
				ThesisContradiction: 0.6,
			},
			{
				ID:                  "dec-exit-1",
				Symbol:              "SOL/USD",
				At:                  exitTime,
				Action:              "exit",
				Reason:              "stoploss: floor breached at 90.00",
				ThesisScore:         0.2,
				ThesisSupport:       0.1,
				ThesisContradiction: 0.9,
			},
		}

		Convey("ExtractLosses should identify and diagnose the losing trade", func() {
			losses := ExtractLosses(decisions, series)

			So(len(losses), ShouldEqual, 1)
			loss := losses[0]
			So(loss.Symbol, ShouldEqual, "SOL/USD")
			So(loss.EntryPrice, ShouldEqual, 100)
			So(loss.ExitPrice, ShouldEqual, 90)
			So(loss.GrossPct, ShouldAlmostEqual, -0.10, 1e-9)
			So(loss.ReturnPct, ShouldBeLessThan, -0.10)
			So(loss.Diagnosis.Category, ShouldEqual, DiagnosisWhipsawStopout)
			So(len(loss.Journal), ShouldEqual, 3)
			So(len(loss.Diagnosis.Blockers), ShouldBeGreaterThan, 0)
			So(loss.Diagnosis.Recommendation.Key, ShouldEqual, "tune_stoploss_buffer")
		})
	})

	Convey("Given a trade with friction drag", t, func() {
		entryTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		exitTime := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)

		series := &Series{
			Symbol: "AVAX/USD",
			Points: []Point{
				{At: entryTime, Price: 100, Bid: 99.5, Ask: 100.5, Friction: 0.01},
				{At: exitTime, Price: 100.5, Bid: 100.0, Ask: 101.0, Friction: 0.01},
			},
		}

		decisions := []Decision{
			{
				ID:          "dec-entry-drag",
				Symbol:      "AVAX/USD",
				At:          entryTime,
				Action:      "enter",
				Reason:      "breakout setup",
				ThesisScore: 0.6,
			},
			{
				ID:          "dec-exit-drag",
				Symbol:      "AVAX/USD",
				At:          exitTime,
				Action:      "exit",
				Reason:      "timeout horizon elapsed",
				ThesisScore: 0.4,
			},
		}

		Convey("ExtractLosses should diagnose friction drag as root cause", func() {
			losses := ExtractLosses(decisions, series)

			So(len(losses), ShouldEqual, 1)
			loss := losses[0]
			So(loss.GrossPct, ShouldAlmostEqual, 0.005, 1e-9)
			So(loss.ReturnPct, ShouldBeLessThan, 0)
			So(loss.Diagnosis.Category, ShouldEqual, DiagnosisFrictionDrag)
			So(loss.Diagnosis.Recommendation.Key, ShouldEqual, "widen_friction_hurdle")
		})
	})

	Convey("Given a profitable trade", t, func() {
		entryTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		exitTime := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)

		series := &Series{
			Symbol: "BTC/USD",
			Points: []Point{
				{At: entryTime, Price: 100, Bid: 99.9, Ask: 100.1, Friction: 0.001},
				{At: exitTime, Price: 120, Bid: 119.9, Ask: 120.1, Friction: 0.001},
			},
		}

		decisions := []Decision{
			{
				ID:     "dec-entry-profit",
				Symbol: "BTC/USD",
				At:     entryTime,
				Action: "enter",
			},
			{
				ID:     "dec-exit-profit",
				Symbol: "BTC/USD",
				At:     exitTime,
				Action: "exit",
			},
		}

		Convey("ExtractLosses should not emit profitable positions", func() {
			losses := ExtractLosses(decisions, series)
			So(len(losses), ShouldEqual, 0)
		})
	})

	Convey("Given a symbol with a single enter decision and no exit", t, func() {
		entryTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		series := &Series{
			Symbol: "LINK/USD",
			Points: []Point{
				{At: entryTime, Price: 100, Bid: 99.9, Ask: 100.1, Friction: 0.002},
				{At: entryTime.Add(30 * time.Second), Price: 95, Bid: 94.9, Ask: 95.1, Friction: 0.002},
			},
		}

		decisions := []Decision{
			{
				ID:     "dec-entry-only",
				Symbol: "LINK/USD",
				At:     entryTime,
				Action: "enter",
			},
		}

		Convey("ExtractLosses should not emit an unclosed entry as a realized loss", func() {
			losses := ExtractLosses(decisions, series)
			So(len(losses), ShouldEqual, 0)
		})
	})
}
