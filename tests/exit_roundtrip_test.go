package tests_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

/*
TestSessionEnterTakeProfitLocksCash proves the simulation stack can:
 1. Find and admit an enter on pump tape
 2. Manage an open lot through Stoploss take_profit when forward edge dies
 3. Submit the exit through Desk/paper and lock quote cash above post-entry

Enter sizing versus paper mark can disagree across emulator tickers, so the
fill is wallet-seeded after the enter Decision (same inventory Desk.adoptOpen
uses on restart). Exit goes fully through Regulate → Trade → paper sell.
*/
func TestSessionEnterTakeProfitLocksCash(t *testing.T) {
	Convey("Given a Session with a stateful paper CLI and a pump ignition", t, func() {
		statePath := filepath.Join(t.TempDir(), "paper-state.json")
		tests.InstallPaperCLI(t, statePath)

		session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
			Signals: pumpdumpSignals,
		})
		So(err, ShouldBeNil)

		_, err = session.Play(conditions.Pump(24, 12, 1.25, 8).Frames())
		So(err, ShouldBeNil)

		So(session.SeedTakerFee("MATIC/USD", 0.26), ShouldBeNil)
		So(session.SeedQuoteCapital(10_000), ShouldBeNil)
		tests.SetPaperCash(t, statePath, 10_000)
		session.Desk().SetSlots(2, 2)

		thesis := types.NewThesis(nil, nil)
		tests.SeedOpportunityForecast(thesis, "MATIC/USD", 0.12, 0.02)
		tests.SeedEarlyCognition(thesis, "MATIC/USD")

		So(session.RunDecide(thesis), ShouldBeNil)

		entered := false

		for _, decision := range thesis.Decisions {
			if decision.Symbol == "MATIC/USD" && decision.Action == "enter" {
				entered = true
			}
		}

		So(entered, ShouldBeTrue)

		const (
			qty   = 100.0
			entry = 1.0
			peak  = 1.08
		)

		// Post-entry wallet: quote spent on the lot, base inventory live.
		cashAfterEnter := 10_000.0 - qty*entry
		So(session.SeedQuoteCapital(cashAfterEnter), ShouldBeNil)
		tests.SetPaperCash(t, statePath, cashAfterEnter)
		tests.SetPaperAsset(t, statePath, "MATIC", qty)
		tests.SetPaperPrice(t, statePath, "MATIC/USD", entry)

		at := time.Unix(1, 0).UTC()
		stop := types.NewStoploss(context.Background())
		stop.Bind(entry, 0.002)
		lot := &types.Holding{
			Symbol:     "MATIC/USD",
			Asset:      "MATIC",
			Qty:        decimal.NewFromFloat64(qty),
			EntryPrice: decimal.NewFromFloat64(entry),
			EntryFee:   decimal.NewFromFloat64(qty * entry * 0.0026),
			Mark:       decimal.NewFromFloat64(entry),
			Status:     types.OPEN,
			EntryAt:    &at,
			Stoploss:   stop,
		}
		session.Balance().Seed(lot)
		So(session.Desk().OpenPositions(), ShouldEqual, 1)

		// Peak then mark-at-peak with dead forward — proximity 0 always qualifies.
		stop.ObserveMark(peak)
		So(markSymbol(session, "MATIC/USD", peak), ShouldBeNil)
		tests.SetPaperPrice(t, statePath, "MATIC/USD", peak)

		exitThesis := types.NewThesis(nil, nil)
		tests.SeedOpportunityForecast(exitThesis, "MATIC/USD", -0.02, 0.01)
		exitThesis.At = at

		So(session.RunTrade(exitThesis), ShouldBeNil)

		tookProfit := false

		for _, decision := range exitThesis.Decisions {
			if decision.Symbol == "MATIC/USD" &&
				decision.Action == types.ActionExit &&
				decision.Cause == "take_profit" {
				tookProfit = true
			}
		}

		So(tookProfit, ShouldBeTrue)

		phase, hasPhase := exitThesis.Lifecycle.Load("MATIC/USD")
		So(hasPhase, ShouldBeTrue)
		So(phase, ShouldBeIn, types.LifecycleExitSubmitted, types.LifecycleClosed)

		flat, flatErr := session.Balance().Holding("MATIC/USD")
		So(flatErr != nil || flat.Status == types.CLOSED ||
			flat.Qty == nil || flat.Qty.Sign() <= 0, ShouldBeTrue)

		cashAfterExit, err := session.Balance().AvailableQuote()
		So(err, ShouldBeNil)
		So(cashAfterExit, ShouldBeGreaterThan, cashAfterEnter)

		Convey("Then take_profit exits through paper and locks quote above entry cash", func() {
			So(session.Desk().OpenPositions(), ShouldEqual, 0)
			So(cashAfterExit, ShouldBeGreaterThan, cashAfterEnter+qty*(peak-entry)*0.5)
		})
	})
}

func markSymbol(session *tests.Session, symbol string, mark float64) error {
	price := decimal.NewFromFloat64(mark).String()
	session.Price().TickerAck([]byte(
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"` + symbol + `","last":"` + price +
			`","bid":"` + price + `","ask":"` + price + `"}]}`,
	))
	session.Desk().Mark(symbol)

	return nil
}
