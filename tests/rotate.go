package tests

import (
	"context"
	"iter"
	"testing"

	"github.com/theapemachine/symm/types"
)

/*
ProveRotateDisplace plays frames, seeds a weak incumbent versus a stronger
challenger, commits strategy, and asserts WEAK exits and MATIC enters by
rotation. Signals and market frames are parameterized so catalog and session
suites share one path.
*/
func ProveRotateDisplace(
	t testing.TB,
	signals SignalFactory,
	frames iter.Seq[Frame],
) {
	t.Helper()

	session, err := NewSession(context.Background(), t, SessionOptions{
		Signals: signals,
	})

	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	if _, err := session.Play(frames); err != nil {
		t.Fatalf("play: %v", err)
	}

	if err := session.SeedTakerFee("MATIC/USD", 0.26); err != nil {
		t.Fatal(err)
	}

	if err := session.SeedTakerFee("WEAK/USD", 0.26); err != nil {
		t.Fatal(err)
	}

	if err := session.SeedQuoteCapital(100); err != nil {
		t.Fatal(err)
	}

	thesis := types.NewThesis(nil, nil)
	session.SeedMatureHolding(thesis, "WEAK/USD", 100)
	SeedOpportunityForecast(thesis, "WEAK/USD", 0.02, 0.01)
	SeedOpportunityForecast(thesis, "MATIC/USD", 0.12, 0.01)
	SeedEarlyCognition(thesis, "MATIC/USD")
	session.Desk.SetSlots(1, 0)

	if err := session.CommitStrategy(thesis); err != nil {
		t.Fatalf("CommitStrategy: %v", err)
	}

	exited, entered := false, false

	for _, decision := range thesis.Decisions {
		if decision.Action == types.ActionExit && decision.Symbol == "WEAK/USD" {
			if decision.Cause != "rotation" {
				t.Fatalf("want rotation exit cause, got %q", decision.Cause)
			}

			exited = true
		}

		if decision.Action == types.ActionEnter && decision.Symbol == "MATIC/USD" {
			if decision.Cause != "rotation" {
				t.Fatalf("want rotation enter cause, got %q", decision.Cause)
			}

			entered = true
		}
	}

	if !exited || !entered {
		t.Fatalf(
			"want WEAK exit and MATIC enter by rotation (exit=%v enter=%v)",
			exited, entered,
		)
	}
}

/*
ProveRotateWait plays frames, seeds a strong incumbent versus a weaker
challenger with no spare quote, commits strategy, and asserts rotate_wait
instead of displacement.
*/
func ProveRotateWait(
	t testing.TB,
	signals SignalFactory,
	frames iter.Seq[Frame],
) {
	t.Helper()

	session, err := NewSession(context.Background(), t, SessionOptions{
		Signals: signals,
	})

	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	if _, err := session.Play(frames); err != nil {
		t.Fatalf("play: %v", err)
	}

	if err := session.SeedTakerFee("MATIC/USD", 0.26); err != nil {
		t.Fatal(err)
	}

	if err := session.SeedTakerFee("HOLD/USD", 0.26); err != nil {
		t.Fatal(err)
	}

	if err := session.SeedQuoteCapital(0); err != nil {
		t.Fatal(err)
	}

	thesis := types.NewThesis(nil, nil)
	session.SeedMatureHolding(thesis, "HOLD/USD", 100)
	SeedOpportunityForecast(thesis, "HOLD/USD", 0.10, 0.01)
	SeedOpportunityForecast(thesis, "MATIC/USD", 0.04, 0.01)
	SeedEarlyCognition(thesis, "MATIC/USD")
	session.Desk.SetSlots(1, 0)

	if err := session.CommitStrategy(thesis); err != nil {
		t.Fatalf("CommitStrategy: %v", err)
	}

	sawWait := false

	for _, decision := range thesis.Decisions {
		if decision.Action == types.ActionExit {
			t.Fatalf("want no exit, got %#v", decision)
		}

		if decision.Symbol == "MATIC/USD" && decision.Action == types.ActionEnter {
			t.Fatal("want no MATIC enter under rotate_wait")
		}

		if decision.Symbol == "MATIC/USD" && decision.Cause == "rotate_wait" {
			sawWait = true
		}
	}

	if !sawWait {
		t.Fatal("want rotate_wait cause on MATIC")
	}
}
