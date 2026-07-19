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

			theses, err := session.Play(Frames(entry))

			if err != nil {
				testingT.Fatalf("play: %v", err)
			}

			if err := AssertMeasure(theses, entry); err != nil {
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

	if _, err := session.Play(Frames(entry)); err != nil {
		t.Fatalf("play: %v", err)
	}

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

	if err := AssertDecide(thesis, entry); err != nil {
		t.Fatal(err)
	}

	after, err := session.Balance.AvailableQuote()

	if err != nil {
		t.Fatal(err)
	}

	if entry.Truth.WalletBound == "deploy" {
		if err := AssertSizedEnter(thesis, entry); err != nil {
			t.Fatal(err)
		}

		return
	}

	if err := AssertWallet(before, after, entry); err != nil {
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
