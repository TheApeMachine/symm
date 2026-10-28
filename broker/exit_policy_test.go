package broker

import (
	"strings"
	"testing"
)

func newTestAdaptivePolicy(hardLoss float64) *AdaptiveExitPolicy {
	config := DefaultAdaptiveExitConfig()
	config.HardLossLimitPct = hardLoss
	return NewAdaptiveExitPolicy(config)
}

func observeExitPolicy(policy *AdaptiveExitPolicy, pnl, friction float64) ExitDecision {
	return policy.Evaluate(ExitObservation{
		PnLPct:      pnl,
		FrictionPct: friction,
	})
}

func TestAdaptivePolicyProtectsAgainstAdverseMove(t *testing.T) {
	t.Parallel()

	policy := newTestAdaptivePolicy(5)
	for _, pnl := range []float64{0, 0.5, 1.0} {
		decision := observeExitPolicy(policy, pnl, 1)
		if decision.Exit {
			t.Fatalf("unexpected exit at PnL %.2f: %s", pnl, decision.Reason)
		}
	}

	decision := observeExitPolicy(policy, -2.1, 1)
	if !decision.Exit || !strings.Contains(decision.Reason, "invalidation") {
		t.Fatalf("expected adaptive loss exit, got %+v", decision)
	}
	if decision.LossFloorPct != -2 {
		t.Fatalf("loss floor = %.3f, want -2.000", decision.LossFloorPct)
	}
}

func TestAdaptivePolicyAllowsNoiseThenTrailsRunner(t *testing.T) {
	t.Parallel()

	policy := newTestAdaptivePolicy(5)
	for _, pnl := range []float64{0, 1, 2, 3, 4, 3} {
		decision := observeExitPolicy(policy, pnl, 1)
		if decision.Exit {
			t.Fatalf("unexpected exit at PnL %.2f: %s", pnl, decision.Reason)
		}
	}
	decision := observeExitPolicy(policy, 2, 1)
	if !decision.Exit || !strings.Contains(decision.Reason, "trailing") {
		t.Fatalf("expected trailing exit at floor, got %+v", decision)
	}
	if decision.TrailFloorPct != 2 {
		t.Fatalf("trail floor = %.3f, want 2.000", decision.TrailFloorPct)
	}
}

func TestAdaptivePolicyLiquidityStressCanExitBeforeTrail(t *testing.T) {
	t.Parallel()

	policy := newTestAdaptivePolicy(5)
	for _, pnl := range []float64{0, 1, 2, 3, 4} {
		decision := observeExitPolicy(policy, pnl, 1)
		if decision.Exit {
			t.Fatalf("unexpected exit at PnL %.2f: %s", pnl, decision.Reason)
		}
	}

	decision := policy.Evaluate(ExitObservation{
		PnLPct:      3,
		FrictionPct: 1,
		ExitRoom:    0.5,
		HasExitRoom: true,
	})
	if !decision.Exit || !decision.LiquidityStress {
		t.Fatalf("expected liquidity stress exit, got %+v", decision)
	}
	if !strings.Contains(decision.Reason, "liquidity stress") {
		t.Fatalf("unexpected reason: %s", decision.Reason)
	}
}

func TestAdaptivePolicyDoesNotTreatUnknownExitRoomAsZero(t *testing.T) {
	t.Parallel()

	policy := newTestAdaptivePolicy(5)
	for _, pnl := range []float64{0, 1, 2, 3, 4} {
		_ = observeExitPolicy(policy, pnl, 1)
	}

	decision := policy.Evaluate(ExitObservation{
		PnLPct:      3,
		FrictionPct: 1,
	})
	if decision.Exit {
		t.Fatalf("unknown exit room must not invent a liquidity failure: %+v", decision)
	}
}

func TestAdaptivePolicyHardLossLimitCapsStepBasedRiskFloor(t *testing.T) {
	t.Parallel()

	policy := newTestAdaptivePolicy(4)
	for _, pnl := range []float64{-1, 2} {
		decision := observeExitPolicy(policy, pnl, 3)
		if decision.Exit {
			t.Fatalf("unexpected exit at PnL %.2f: %s", pnl, decision.Reason)
		}
	}
	decision := observeExitPolicy(policy, -4.1, 3)
	if !decision.Exit || !strings.Contains(decision.Reason, "invalidation") {
		t.Fatalf("expected configured hard-loss exit, got %+v", decision)
	}
	if decision.LossFloorPct != -4 {
		t.Fatalf("loss floor = %.3f, want -4.000", decision.LossFloorPct)
	}
}

func TestAdaptivePolicyFloorOnlyMovesUpAfterTrailingArms(t *testing.T) {
	t.Parallel()

	policy := newTestAdaptivePolicy(5)
	for _, pnl := range []float64{0, 1, 2, 3, 4} {
		_ = observeExitPolicy(policy, pnl, 1)
	}
	first := policy.trailFloorPct

	second := observeExitPolicy(policy, 5, 1)
	if second.Exit {
		t.Fatalf("unexpected exit at new peak: %+v", second)
	}
	if second.TrailFloorPct <= first {
		t.Fatalf("floor did not ratchet upward: first %.2f, next %.2f", first, second.TrailFloorPct)
	}
}
