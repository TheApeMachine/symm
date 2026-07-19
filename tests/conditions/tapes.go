package conditions

import "github.com/theapemachine/symm/tests"

/*
TapeCalm is a multi-leg calm baseline for contrast against opportunity tapes.
*/
func TapeCalm() *tests.Market {
	return Calm(32)
}

/*
TapePump is a multi-leg pump: calm setup, vertical ignition, sustained volume.
*/
func TapePump() *tests.Market {
	return Pump(32, 12, 1.25, 8)
}

/*
TapeCoil compresses then breaks — longer horizon ignition after setup.
*/
func TapeCoil() *tests.Market {
	return Pump(40, 28, 1.18, 6)
}

/*
TapeExhaustion withdraws book depth into mechanical exhaustion structure.
*/
func TapeExhaustion() *tests.Market {
	return Decay(32, 0, 0.9)
}

/*
TapeVacuum withdraws resting depth while the tape continues.
*/
func TapeVacuum() *tests.Market {
	return Decay(32, 8, 0.85)
}

/*
TapeSectorLift is a co-moving herd cohort (sector breadth).
*/
func TapeSectorLift() *tests.Market {
	return Herd(32)
}

/*
TapeThinBook starves subject touch size inside a herd.
*/
func TapeThinBook() *tests.Market {
	return ThinHerd(32, 0.05)
}

/*
TapeNoise is unstructured multi-symbol motion.
*/
func TapeNoise() *tests.Market {
	return Noise(32)
}

/*
TapePhantomQuote retreats quotes without trade confirmation.
*/
func TapePhantomQuote() *tests.Market {
	return PhantomDrawdown(32, 10, 0.04)
}

/*
TapeToxicChase is buy aggression without supportive book. qtyMul is high enough
that Level3 fill evidence diverges from calm SeedTouch baselines.
*/
func TapeToxicChase() *tests.Market {
	return Aggression(32, 0, 20)
}

/*
TapePumpThenReversal ignites then flips.
*/
func TapePumpThenReversal() *tests.Market {
	return Reversal(36, 20, 0.02)
}

/*
TapeLag is follower lag without a tradeable lead claim for the subject.
*/
func TapeLag() *tests.Market {
	return Lag(32, 4)
}

/*
TapeShallowAdverse is a mild quote retreat (~0.8%) for exit-honesty holds —
the audit-trail band where exit-happy stops used to fire.
*/
func TapeShallowAdverse() *tests.Market {
	return PhantomDrawdown(32, 10, 0.008)
}

/*
TapeDrawdownStop is a sincere multi-leg drawdown for stop Regulate proofs.
*/
func TapeDrawdownStop() *tests.Market {
	return Drawdown(32, 0.12, 10)
}

/*
TapeCalibratedLift is a modest marked-up tape for locked-floor hold proofs.
*/
func TapeCalibratedLift() *tests.Market {
	return CalibratedLift(32, 8, 1.04)
}

/*
TapeImbalance is a bid-loaded book for depthflow loaded-score proofs.
*/
func TapeImbalance() *tests.Market {
	return Imbalance(32, 0, 6, 0.15)
}

/*
TapeBalancedBook is a two-sided balanced book contrast for depthflow.
*/
func TapeBalancedBook() *tests.Market {
	return Imbalance(32, 0, 1, 1)
}
