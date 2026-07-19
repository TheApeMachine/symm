package catalog

import (
	"context"
	"fmt"
	"testing"

	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
ForKind returns catalog entries matching kind (usually one).
*/
func ForKind(kind ScenarioKind) []Entry {
	out := make([]Entry, 0)

	for _, entry := range All() {
		if entry.Kind == kind {
			out = append(out, entry)
		}
	}

	return out
}

/*
MustKind returns the first entry for kind or fails the test.
*/
func MustKind(t testing.TB, kind ScenarioKind) Entry {
	t.Helper()

	entries := ForKind(kind)

	if len(entries) == 0 {
		t.Fatalf("catalog: no entry for kind %s", kind)
	}

	return entries[0]
}

/*
ProveMeasure boots the stack with signals, plays each kind entry's tape through
Crypto.Tick, and asserts that entry's measure truth.
*/
func ProveMeasure(
	testingT *testing.T,
	kind ScenarioKind,
	signals tests.SignalFactory,
) {
	testingT.Helper()

	entries := ForKind(kind)

	if len(entries) == 0 {
		testingT.Fatalf("catalog: no entry for kind %s", kind)
	}

	for _, entry := range entries {
		entry := entry

		if entry.Truth.MeasureSource == "" {
			continue
		}

		testingT.Run(entry.Name, func(testingT *testing.T) {
			session, err := tests.NewSession(
				context.Background(), testingT, tests.SessionOptions{Signals: signals},
			)

			if err != nil {
				testingT.Fatalf("boot: %v", err)
			}

			theses, err := session.Play(entry.Frames())

			if err != nil {
				testingT.Fatalf("play: %v", err)
			}

			if err := entry.AssertMeasure(theses); err != nil {
				testingT.Fatal(err)
			}
		})
	}
}

/*
ProveStrategy seeds capital/fees/forecasts as labeled, runs CommitStrategy, and
asserts decide + wallet stage truths for the kind.
*/
func ProveStrategy(
	t testing.TB,
	kind ScenarioKind,
	signals tests.SignalFactory,
) {
	t.Helper()

	entry := MustKind(t, kind)
	session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
		Signals: signals,
	})

	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	if _, err := session.Play(entry.Frames()); err != nil {
		t.Fatalf("play: %v", err)
	}

	ProveStrategyOnSession(t, session, entry)
}

/*
ProveStrategyOnSession seeds fees/capital, commits strategy, and asserts decide
+ wallet truths on an already-played session. Shared by ProveStrategy and the
single-session lifecycle proofs.
*/
func ProveStrategyOnSession(t testing.TB, session *tests.Session, entry Entry) {
	t.Helper()

	if err := session.SeedTakerFee(entry.Symbol, entry.FeePct); err != nil {
		t.Fatal(err)
	}

	if err := session.SeedQuoteCapital(entry.Capital); err != nil {
		t.Fatal(err)
	}

	before, err := session.Balance.AvailableQuote()

	if err != nil {
		t.Fatal(err)
	}

	session.Desk.SetSlots(2, 2)
	thesis := types.NewThesis(nil, nil)

	if entry.Truth.DecideAction == types.ActionEnter ||
		entry.Truth.SizedEnter ||
		entry.Kind == KindUnfundableLot {
		tests.SeedOpportunityForecast(thesis, entry.Symbol, 0.12, 0.02)
		tests.SeedEarlyCognition(thesis, entry.Symbol)
	}

	if err := session.CommitStrategy(thesis); err != nil {
		t.Fatalf("CommitStrategy: %v", err)
	}

	if err := entry.AssertDecide(thesis); err != nil {
		t.Fatal(err)
	}

	after, err := session.Balance.AvailableQuote()

	if err != nil {
		t.Fatal(err)
	}

	if entry.Truth.WalletBound == WalletBoundDeploy {
		if err := AssertSizedEnter(thesis, entry); err != nil {
			t.Fatal(err)
		}
	}

	if err := entry.AssertWallet(before, after); err != nil {
		t.Fatal(err)
	}
}

/*
AssertSizedEnter requires a positive ProposedNotional enter for the entry symbol.
*/
func AssertSizedEnter(thesis *types.Thesis, entry Entry) error {
	if thesis == nil {
		return errSized(entry.Name, "nil thesis")
	}

	for _, decision := range thesis.Decisions {
		if decision.Symbol == entry.Symbol &&
			decision.Action == types.ActionEnter &&
			decision.ProposedNotional != nil &&
			decision.ProposedNotional.Sign() > 0 {
			return nil
		}
	}

	return errSized(entry.Name, "no sized enter")
}

func errSized(name, detail string) error {
	return fmt.Errorf("catalog %s: %s", name, detail)
}

/*
ProveExit boots the stack, PlayOpens the catalog tape with a bound lot, applies
the entry's mark/retreat surface, CommitStrategy-regulates, and asserts exit
honesty (hold under phantom/shallow adverse; stop on sincere breach).
*/
func ProveExit(
	t testing.TB,
	kind ScenarioKind,
	signals tests.SignalFactory,
) {
	t.Helper()

	entry := MustKind(t, kind)

	if !entry.IsExitProof() {
		t.Fatalf("catalog %s: not an exit proof entry", entry.Name)
	}

	session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
		Signals: signals,
	})

	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	trail := entry.Truth.TrailDistance

	if trail <= 0 {
		t.Fatalf(
			"catalog %s: TrailDistance > 0 required for exit proof (no magic default)",
			entry.Name,
		)
	}

	if entry.Market == nil {
		t.Fatalf("catalog %s: market tape required for exit proof", entry.Name)
	}

	lot, statePath, err := session.PlayOpen(t, entry.Market(), entry.Symbol, 100, trail)

	if err != nil {
		t.Fatalf("PlayOpen: %v", err)
	}

	entryPrice := lot.EntryPrice.Float64()

	if entry.Truth.PeakMul > 1 {
		peak := entryPrice * entry.Truth.PeakMul

		if err := session.ObserveQuote(lot, peak, peak); err != nil {
			t.Fatal(err)
		}

		tests.SetPaperPrice(t, statePath, entry.Symbol, peak)
	}

	markMul := entry.Truth.MarkMul

	if markMul <= 0 {
		t.Fatalf("catalog %s: MarkMul required for exit proof", entry.Name)
	}

	mark := entryPrice * markMul

	if entry.Truth.RetreatPressure > 0 {
		lot.Stoploss.NoteRetreat(entry.Truth.RetreatPressure)
	}

	if err := session.ObserveQuote(lot, mark, mark); err != nil {
		t.Fatal(err)
	}

	tests.SetPaperPrice(t, statePath, entry.Symbol, mark)

	thesis := types.NewThesis(nil, nil)
	er, unc := forecastSeed(entry.Truth)

	if entry.Truth.KeepForecast {
		tests.SeedOpportunityForecast(thesis, entry.Symbol, er, unc)
		thesis.Forecasts[len(thesis.Forecasts)-1].IncrementalMSE = unc * unc * 0.01
	}

	if entry.Truth.RetreatPressure > 0 {
		tests.SeedRetreat(thesis, entry.Symbol, entry.Truth.RetreatPressure)
	}

	if err := session.CommitStrategy(thesis); err != nil {
		t.Fatalf("CommitStrategy: %v", err)
	}

	if entry.Truth.StickyRetreat {
		// Second cut without retreat measurements — sticky pressure must still gate.
		sticky := types.NewThesis(nil, nil)

		if entry.Truth.KeepForecast {
			tests.SeedOpportunityForecast(sticky, entry.Symbol, er, unc)
		}

		if err := session.CommitStrategy(sticky); err != nil {
			t.Fatalf("CommitStrategy sticky: %v", err)
		}

		thesis = sticky
	}

	if err := entry.AssertExit(thesis, lot); err != nil {
		t.Fatal(err)
	}

	if entry.Truth.MustNotExit && session.Desk.OpenPositions() != 1 {
		t.Fatalf(
			"catalog %s: want open lot retained, open=%d",
			entry.Name, session.Desk.OpenPositions(),
		)
	}
}

/*
forecastSeed applies StageTruth ER/uncertainty defaults used by KeepForecast
paths: 0.05 for zero ER and 0.02 for nonpositive uncertainty.
*/
func forecastSeed(truth StageTruth) (er, unc float64) {
	er = truth.ForecastER
	unc = truth.ForecastUnc

	if er == 0 {
		er = 0.05
	}

	if unc <= 0 {
		unc = 0.02
	}

	return er, unc
}
