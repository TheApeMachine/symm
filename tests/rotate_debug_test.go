package tests_test

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

func TestDebugRotateDecisions(t *testing.T) {
	session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
		Signals: pumpdumpSignals,
	})

	if err != nil {
		t.Fatal(err)
	}

	if _, err := session.Play(conditions.Pump(24, 12, 1.25, 8).Frames()); err != nil {
		t.Fatal(err)
	}

	_ = session.SeedTakerFee("MATIC/USD", 0.26)
	_ = session.SeedTakerFee("WEAK/USD", 0.26)
	_ = session.SeedQuoteCapital(0)

	thesis := types.NewThesis(nil, nil)
	tests.SeedMatureHolding(thesis, "WEAK/USD", 100)
	tests.SeedOpportunityForecast(thesis, "WEAK/USD", 0.02, 0.01)
	tests.SeedOpportunityForecast(thesis, "MATIC/USD", 0.12, 0.01)
	tests.SeedEarlyCognition(thesis, "MATIC/USD")
	session.Desk.SetSlots(1, 0)

	if err := session.RunDecide(thesis); err != nil {
		t.Fatal(err)
	}

	for _, decision := range thesis.Decisions {
		t.Logf(
			"decision action=%s symbol=%s cause=%s class=%s util=%v margin=%v",
			decision.Action, decision.Symbol, decision.Cause,
			decision.AllocationClass, decision.Utility, decision.OpportunityMargin,
		)
	}

	t.Logf("open=%d decisions=%d", session.Desk.OpenPositions(), len(thesis.Decisions))
}
