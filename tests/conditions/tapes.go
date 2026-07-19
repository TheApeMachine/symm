package conditions

import "github.com/theapemachine/symm/tests"

/*
TapeCalm is a multi-leg calm baseline for contrast against opportunity tapes.
*/
func TapeCalm() *tests.Market {
	return Calm(32)
}

/*
TapePumpDump is a multi-leg calibration, ignition, continuation, and rejection
tape whose ticker volume follows Kraken's cumulative-volume semantics.
*/
func TapePumpDump() *tests.Market {
	return PumpDump()
}

/*
TapeExhaustion withdraws book depth into mechanical exhaustion structure.
*/
func TapeExhaustion() *tests.Market {
	return Decay(32, 0, 0.9)
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
TapeAlpha is a peer-aligned subject path with distinctly greater return energy.
*/
func TapeAlpha() *tests.Market {
	return Alpha(32)
}

/*
TapeDivergence raises the subject while the peer cohort falls.
*/
func TapeDivergence() *tests.Market {
	return Divergence(32)
}

/*
TapeSlump lowers the entire cohort without manufacturing a standout leader.
*/
func TapeSlump() *tests.Market {
	return Slump(32)
}

/*
TapeStall establishes a leader and then stops it while peers keep moving.
*/
func TapeStall() *tests.Market {
	return Stall(48)
}

/*
TapeToxicChase is buy aggression without supportive book.
*/
func TapeToxicChase() *tests.Market {
	return Aggression(32, 0, 20)
}

/*
TapeLag is follower lag without a tradeable lead claim for the subject.
*/
func TapeLag() *tests.Market {
	return Lag(32, 4)
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
