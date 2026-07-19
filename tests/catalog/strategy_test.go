package catalog_test

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/catalog"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

/*
TestCatalogStrategyFundedSliceSizesEnter proves Allocator sizing on the real
stack for the funded_slice catalog truth (seeded forecast control surface).
*/
func TestCatalogStrategyFundedSliceSizesEnter(t *testing.T) {
	catalog.ProveStrategy(t, catalog.KindFundedSlice, catalog.Signals)
}

/*
TestCatalogStrategyUnfundableRefusesEnter proves min-lot / wallet refuse.
*/
func TestCatalogStrategyUnfundableRefusesEnter(t *testing.T) {
	catalog.ProveStrategy(t, catalog.KindUnfundableLot, catalog.Signals)
}

/*
TestCatalogStrategyRotateDisplacesWeakest proves rotation exit+enter through
stack.Boot + Play + CommitStrategy (strategy-given-forecast labeled).
*/
func TestCatalogStrategyRotateDisplacesWeakest(t *testing.T) {
	session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
		Signals: catalog.Signals,
	})

	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	if _, err := session.Play(conditions.TapePump().Frames()); err != nil {
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
	tests.SeedOpportunityForecast(thesis, "WEAK/USD", 0.02, 0.01)
	tests.SeedOpportunityForecast(thesis, "MATIC/USD", 0.12, 0.01)
	tests.SeedEarlyCognition(thesis, "MATIC/USD")
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
		t.Fatalf("want WEAK exit and MATIC enter by rotation (exit=%v enter=%v)", exited, entered)
	}
}

/*
TestCatalogStrategyRotateWaitsWhenSurplusNonPositive proves rotate_wait.
*/
func TestCatalogStrategyRotateWaitsWhenSurplusNonPositive(t *testing.T) {
	session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
		Signals: catalog.Signals,
	})

	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	if _, err := session.Play(conditions.Pump(16, 8, 1.2, 4).Frames()); err != nil {
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
	tests.SeedOpportunityForecast(thesis, "HOLD/USD", 0.10, 0.01)
	tests.SeedOpportunityForecast(thesis, "MATIC/USD", 0.04, 0.01)
	tests.SeedEarlyCognition(thesis, "MATIC/USD")
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

/*
TestCatalogStrategyRefuseTraps proves MustNotEnter kinds do not size enters
when strategy is committed without a seeded opportunity forecast (wallet preserve).
*/
func TestCatalogStrategyRefuseTraps(t *testing.T) {
	for _, kind := range []catalog.ScenarioKind{
		catalog.KindExhaustion,
		catalog.KindNoise,
		catalog.KindThinBook,
	} {
		kind := kind
		t.Run(string(kind), func(t *testing.T) {
			catalog.ProveStrategy(t, kind, catalog.Signals)
		})
	}
}

func BenchmarkCatalogStrategyFundedSlice(b *testing.B) {
	entry := catalog.MustKind(b, catalog.KindFundedSlice)
	session, err := tests.NewSession(context.Background(), b, tests.SessionOptions{
		Signals: catalog.Signals,
	})

	if err != nil {
		b.Fatal(err)
	}

	frames := catalog.Frames(entry)
	b.ReportAllocs()

	for b.Loop() {
		if _, err := session.Play(frames); err != nil {
			b.Fatal(err)
		}

		if err := session.SeedTakerFee(entry.Symbol, entry.FeePct); err != nil {
			b.Fatal(err)
		}

		if err := session.SeedQuoteCapital(entry.Capital); err != nil {
			b.Fatal(err)
		}

		session.Desk.SetSlots(2, 2)
		thesis := types.NewThesis(nil, nil)
		tests.SeedOpportunityForecast(thesis, entry.Symbol, 0.12, 0.02)
		tests.SeedEarlyCognition(thesis, entry.Symbol)

		if err := session.CommitStrategy(thesis); err != nil {
			b.Fatal(err)
		}

		if err := catalog.AssertSizedEnter(thesis, entry); err != nil {
			b.Fatal(err)
		}
	}
}
