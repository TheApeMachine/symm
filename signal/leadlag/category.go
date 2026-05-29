package leadlag

import "github.com/theapemachine/symm/engine"

/*
leadlagCategory maps anchor/follower lag structure onto the anchor perspective.
*/
func leadlagCategory(
	correlation float64,
	lagFraction float64,
	anchorMoved bool,
	reason string,
) engine.Category {
	if !anchorMoved || reason == "anchor_stall" {
		return engine.CatAnchorStall
	}

	if reason == "decoupled" || correlation < leadlagMinimumLagCorrelation {
		return engine.CatDecoupledMove
	}

	if lagFraction >= minLagFraction {
		return engine.CatInefficientLag
	}

	return engine.CatSynchronizedDrift
}
