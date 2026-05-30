package perspectives

/*
NewScarcityPerspective builds the thin-market convexity playbook (DECISION.md
stage 4 timing fused with the Liquidity "Scarcity" perspective).

The idea is convexity: in the thinnest pipe in the universe a small amount of order
flow causes the largest price displacement. CategoryExtremeScarcity (Liquidity:
peak illiquidity versus peers) is a hard gate — it carries no verdict alone, because
scarcity without a catalyst is just a quiet coin. The catalyst is ignition:
CategoryVerticalIgnition (already launching) or CategoryCoiledCompression (wound and
ready to snap) each authorize entry beneath the scarcity gate, so a tiny position
rides an outsized move. Two ignition flavours sit as siblings so either path opens
the trade, and the deeper of the reachable leaves wins when both are present.

Held, the convex move is fragile by construction: CategoryActiveReversal is the
urgent stop, and CategoryFadedExhaustion (the leg is dead, lift has fallen away)
harvests before the thin book gaps back.
*/
func NewScarcityPerspective() *strategy {
	return newStrategy(RegimeChoppy, scarcityBranches)
}

var scarcityBranches = []Branch{
	snrGate(CategoryExtremeScarcity,
		entryBranch(CategoryVerticalIgnition,
			holdingExitBranch(CategoryActiveReversal, CategoryFadedExhaustion),
		),
		entryBranch(CategoryCoiledCompression,
			holdingExitBranch(CategoryActiveReversal, CategoryFadedExhaustion),
		),
	),
}
