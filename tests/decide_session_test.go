package tests_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

/*
TestSessionPumpReservedDoesNotRotateMaturing plays a controlled pump through
the Kraken Conn emulator, then runs Decide with a reserved-class ignition and
a maturing incumbent. The pump name must take reserved overflow without
exiting the incumbent.
*/
func TestSessionPumpReservedDoesNotRotateMaturing(t *testing.T) {
	Convey("Given an emulator Session on a pump timeline", t, func() {
		session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
			Signals: pumpdumpSignals,
		})
		So(err, ShouldBeNil)

		theses, err := session.Play(conditions.Pump(24, 12, 1.25, 8).Frames())
		So(err, ShouldBeNil)
		So(len(theses), ShouldBeGreaterThan, 0)

		calm, hasCalm := tests.PeakMetric(
			theses, "MATIC/USD", types.MetricRVOL,
		)
		So(hasCalm, ShouldBeTrue)
		So(calm, ShouldBeGreaterThan, 0)

		So(session.SeedTakerFee("MATIC/USD", 0.26), ShouldBeNil)
		So(session.SeedTakerFee("BTC/USD", 0.26), ShouldBeNil)
		So(session.SeedQuoteCapital(10_000), ShouldBeNil)

		thesis := types.NewThesis(nil, nil)
		session.SeedMatureHolding(thesis, "BTC/USD", 100)
		tests.SeedOpportunityForecast(thesis, "BTC/USD", 0.03, 0.01)
		tests.SeedOpportunityForecast(thesis, "MATIC/USD", 0.12, 0.02)
		tests.SeedEarlyCognition(thesis, "MATIC/USD")
		thesis.Cognition.Store("BTC/USD", types.Cognition{
			Source: "dmt", Symbol: "BTC/USD", Winner: "buy",
			Ready: true, Confidence: 0.6, Ambiguous: false,
		})

		// One normal slot occupied by the maturing name; reserved stays free.
		session.Desk.SetSlots(1, 2)
		So(session.RunDecide(thesis), ShouldBeNil)

		Convey("Then the pump ignition enters reserved without rotating BTC", func() {
			entered := false
			rotatedBTC := false

			for _, decision := range thesis.Decisions {
				if decision.Symbol == "MATIC/USD" && decision.Action == "enter" {
					entered = true
					So(decision.AllocationClass, ShouldEqual, "reserved")
					So(decision.OpportunityMargin, ShouldBeGreaterThan, 0)
					So(decision.CognitiveLead, ShouldBeGreaterThan, 0)
				}

				if decision.Symbol == "BTC/USD" && decision.Action == "exit" {
					rotatedBTC = true
				}
			}

			So(entered, ShouldBeTrue)
			So(rotatedBTC, ShouldBeFalse)

			holding, ok := findThesisHolding(thesis, "MATIC/USD")
			So(ok, ShouldBeTrue)
			So(holding.IsOpportunity, ShouldBeTrue)
		})
	})
}

/*
TestSessionPumpRotateWhenChallengerClearsWeakest uses the emulator pump cut,
then Decide-displaces a weak open hold when rotate surplus is positive.
*/
func TestSessionPumpRotateWhenChallengerClearsWeakest(t *testing.T) {
	Convey("Given a full normal book and a stronger emulator-backed challenger", t, func() {
		session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
			Signals: pumpdumpSignals,
		})
		So(err, ShouldBeNil)

		_, err = session.Play(conditions.Pump(24, 12, 1.25, 8).Frames())
		So(err, ShouldBeNil)

		So(session.SeedTakerFee("MATIC/USD", 0.26), ShouldBeNil)
		So(session.SeedTakerFee("WEAK/USD", 0.26), ShouldBeNil)
		So(session.SeedQuoteCapital(0), ShouldBeNil)

		thesis := types.NewThesis(nil, nil)
		session.SeedMatureHolding(thesis, "WEAK/USD", 100)
		tests.SeedOpportunityForecast(thesis, "WEAK/USD", 0.02, 0.01)
		tests.SeedOpportunityForecast(thesis, "MATIC/USD", 0.12, 0.01)
		tests.SeedEarlyCognition(thesis, "MATIC/USD")

		session.Desk.SetSlots(1, 0)
		So(session.RunDecide(thesis), ShouldBeNil)

		Convey("Then WEAK is displaced and MATIC enters by rotation", func() {
			exited := false
			entered := false

			for _, decision := range thesis.Decisions {
				if decision.Action == "exit" && decision.Symbol == "WEAK/USD" {
					So(decision.Cause, ShouldEqual, "rotation")
					exited = true
				}

				if decision.Action == "enter" && decision.Symbol == "MATIC/USD" {
					So(decision.Cause, ShouldEqual, "rotation")
					entered = true
				}
			}

			So(exited, ShouldBeTrue)
			So(entered, ShouldBeTrue)
		})
	})
}

/*
TestSessionPumpWaitsWhenRotateSurplusNonPositive keeps a strong maturing hold
when the pump challenger does not clear hold utility plus exit cost.
*/
func TestSessionPumpWaitsWhenRotateSurplusNonPositive(t *testing.T) {
	Convey("Given a strong incumbent and a weaker pump challenger", t, func() {
		session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
			Signals: pumpdumpSignals,
		})
		So(err, ShouldBeNil)

		_, err = session.Play(conditions.Pump(16, 8, 1.2, 4).Frames())
		So(err, ShouldBeNil)

		So(session.SeedTakerFee("MATIC/USD", 0.26), ShouldBeNil)
		So(session.SeedTakerFee("HOLD/USD", 0.26), ShouldBeNil)
		So(session.SeedQuoteCapital(0), ShouldBeNil)

		thesis := types.NewThesis(nil, nil)
		session.SeedMatureHolding(thesis, "HOLD/USD", 100)
		tests.SeedOpportunityForecast(thesis, "HOLD/USD", 0.10, 0.01)
		tests.SeedOpportunityForecast(thesis, "MATIC/USD", 0.04, 0.01)
		tests.SeedEarlyCognition(thesis, "MATIC/USD")

		session.Desk.SetSlots(1, 0)
		So(session.RunDecide(thesis), ShouldBeNil)

		Convey("Then Decide waits instead of rotating", func() {
			sawWait := false

			for _, decision := range thesis.Decisions {
				So(decision.Action, ShouldNotEqual, "exit")

				if decision.Symbol == "MATIC/USD" && decision.Action == "enter" {
					So(false, ShouldBeTrue)
				}

				if decision.Symbol == "MATIC/USD" && decision.Cause == "rotate_wait" {
					sawWait = true
				}
			}

			So(sawWait, ShouldBeTrue)
		})
	})
}

/*
TestSessionRunDecideUsesEmulatorFeesAndCapital exercises Crypto.Decide after
Play so friction and quote capital come from the Conn-backed broker surfaces.
*/
func TestSessionRunDecideUsesEmulatorFeesAndCapital(t *testing.T) {
	Convey("Given Played tape plus seeded fee and balance frames", t, func() {
		session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
			Signals: pumpdumpSignals,
		})
		So(err, ShouldBeNil)

		_, err = session.Play(conditions.Calm(8).Frames())
		So(err, ShouldBeNil)

		So(session.SeedTakerFee("MATIC/USD", 0.26), ShouldBeNil)
		So(session.SeedQuoteCapital(5_000), ShouldBeNil)

		available, err := session.Balance.AvailableQuote()
		So(err, ShouldBeNil)
		So(available, ShouldEqual, 5_000)

		thesis := types.NewThesis(nil, nil)
		tests.SeedOpportunityForecast(thesis, "MATIC/USD", 0.12, 0.02)
		tests.SeedEarlyCognition(thesis, "MATIC/USD")
		// FrictionReady forecasts still need Price fee application via Decide.
		thesis.Forecasts[0].FrictionReady = false

		So(session.RunDecide(thesis), ShouldBeNil)

		Convey("Then Crypto.Decide applies fees and admits the ignition", func() {
			So(thesis.Forecasts[0].FrictionReady, ShouldBeTrue)
			So(thesis.Forecasts[0].ExpectedFees, ShouldEqual, 0.0026)

			entered := false

			for _, decision := range thesis.Decisions {
				if decision.Symbol == "MATIC/USD" && decision.Action == "enter" {
					entered = true
					So(decision.AllocationClass, ShouldEqual, "reserved")
				}
			}

			So(entered, ShouldBeTrue)
		})
	})
}

/*
BenchmarkSessionPumpDecide measures Play + Decide on a pump condition.
*/
func BenchmarkSessionPumpDecide(b *testing.B) {
	session, err := tests.NewSession(context.Background(), b, tests.SessionOptions{
		Signals: pumpdumpSignals,
	})

	if err != nil {
		b.Fatal(err)
	}

	if err := session.SeedTakerFee("MATIC/USD", 0.26); err != nil {
		b.Fatal(err)
	}

	if err := session.SeedQuoteCapital(10_000); err != nil {
		b.Fatal(err)
	}

	session.Desk.SetSlots(2, 2)
	b.ReportAllocs()

	for b.Loop() {
		theses, playErr := session.Play(conditions.Pump(16, 8, 1.25, 6).Frames())

		if playErr != nil {
			b.Fatal(playErr)
		}

		thesis := types.NewThesis(nil, nil)
		tests.SeedOpportunityForecast(thesis, "MATIC/USD", 0.12, 0.02)
		tests.SeedEarlyCognition(thesis, "MATIC/USD")
		_ = theses

		if decideErr := session.RunDecide(thesis); decideErr != nil {
			b.Fatal(decideErr)
		}
	}
}

/*
TestSessionRunTradeHoldsUnderRetreat gates ObserveQuote under NoteRetreat.
*/
func TestSessionRunTradeHoldsUnderRetreat(t *testing.T) {
	Convey("Given a Session with a fee-thin open lot noting retreat", t, func() {
		symbol := conditions.Subject()
		session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
			Signals: pumpdumpSignals,
		})
		So(err, ShouldBeNil)
		lot, statePath, err := session.PlayOpen(
			t, conditions.PhantomDrawdown(24, 8, 0.015), symbol, 100, 0.0026,
		)
		So(err, ShouldBeNil)
		lot.Stoploss.NoteRetreat(0.95)
		entry := lot.EntryPrice.Float64()
		So(session.ObserveQuote(lot, entry*(1-0.015), entry), ShouldBeNil)
		tests.SetPaperPrice(t, statePath, symbol, entry*(1-0.015))

		Convey("When RunTrade regulates under SeedRetreat", func() {
			exitThesis := types.NewThesis(nil, nil)
			tests.SeedOpportunityForecast(exitThesis, symbol, 0.05, 0.02)
			tests.SeedRetreat(exitThesis, symbol, 0.95)
			exitThesis.Forecasts[len(exitThesis.Forecasts)-1].IncrementalMSE =
				0.02 * 0.02 * 0.01
			So(session.RunTrade(exitThesis), ShouldBeNil)

			Convey("Then no exit Decision fires and markReturn is adverse", func() {
				So(exitCause(exitThesis, symbol), ShouldEqual, "")
				So(session.Desk.OpenPositions(), ShouldEqual, 1)
				So(lot.Stoploss.MarkReturn, ShouldBeLessThan, 0)
				So(lot.Stoploss.Action, ShouldEqual, "hold")
			})
		})
	})
}

/*
TestSessionRunTradeStopsWithoutRetreat is the ungated ObserveQuote control.
*/
func TestSessionRunTradeStopsWithoutRetreat(t *testing.T) {
	Convey("Given a Session with a fee-thin open lot and no retreat", t, func() {
		symbol := conditions.Subject()
		session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
			Signals: pumpdumpSignals,
		})
		So(err, ShouldBeNil)
		lot, statePath, err := session.PlayOpen(
			t, conditions.PhantomDrawdown(24, 8, 0.015), symbol, 100, 0.0026,
		)
		So(err, ShouldBeNil)
		entry := lot.EntryPrice.Float64()
		So(session.ObserveQuote(lot, entry*(1-0.015), entry), ShouldBeNil)
		tests.SetPaperPrice(t, statePath, symbol, entry*(1-0.015))

		Convey("When RunTrade regulates without forecast σ", func() {
			exitThesis := types.NewThesis(nil, nil)
			So(session.RunTrade(exitThesis), ShouldBeNil)

			Convey("Then Stoploss.Regulate emits Cause=stop", func() {
				So(exitCause(exitThesis, symbol), ShouldEqual, "stop")
			})
		})
	})
}

/*
TestSessionRunTradeLocksFloorOnCalibratedMark locks floor under ER=5% σ=2%.
*/
func TestSessionRunTradeLocksFloorOnCalibratedMark(t *testing.T) {
	Convey("Given a Session with an open lot on a calibrated lift tape", t, func() {
		symbol := conditions.Subject()
		session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
			Signals: pumpdumpSignals,
		})
		So(err, ShouldBeNil)
		lot, statePath, err := session.PlayOpen(
			t, conditions.CalibratedLift(24, 8, 1.04), symbol, 100, 0.02,
		)
		So(err, ShouldBeNil)
		mark := lot.EntryPrice.Float64() * 1.04
		So(session.ObserveQuote(lot, mark, mark), ShouldBeNil)
		tests.SetPaperPrice(t, statePath, symbol, mark)

		Convey("When RunTrade regulates a calibrated forecast", func() {
			exitThesis := types.NewThesis(nil, nil)
			tests.SeedOpportunityForecast(exitThesis, symbol, 0.05, 0.02)
			exitThesis.Forecasts[len(exitThesis.Forecasts)-1].IncrementalMSE =
				0.02 * 0.02 * 0.01
			So(session.RunTrade(exitThesis), ShouldBeNil)

			Convey("Then LockedFloor is positive and no exit fires", func() {
				So(exitCause(exitThesis, symbol), ShouldEqual, "")
				So(lot.Stoploss.LockedFloor, ShouldBeGreaterThan, 0)
				So(lot.Stoploss.PeakReturn, ShouldAlmostEqual, 0.04, 0.0001)
			})
		})
	})
}

func exitCause(thesis *types.Thesis, symbol string) string {
	for _, decision := range thesis.Decisions {
		if decision.Symbol == symbol && decision.Action == types.ActionExit {
			return decision.Cause
		}
	}

	return ""
}

func findThesisHolding(thesis *types.Thesis, symbol string) (types.Holding, bool) {
	var found types.Holding
	ok := false

	thesis.Holdings.Range(func(key, value any) bool {
		holding, typed := value.(*types.Holding)

		if !typed || holding == nil || holding.Symbol != symbol {
			return true
		}

		found = *holding
		ok = true

		return false
	})

	return found, ok
}
