package perspectives

/*
NewLeadLagPerspective builds the anchor catch-up playbook (DECISION.md stage 2,
node 4 — the LeadLag "Anchor" perspective).

The opportunity is a follower that statistically tracks the leader (BTC/EUR) but has
not yet completed the leader's move: CategoryInefficientLag is a high-correlation
reading with a large unfinished lag fraction — the "catch-up" the LeadLag signal is
built to find. The lag itself is the edge, so it authorizes entry directly; the
signal only emits this category when the anchor has actually moved, so there is no
separate move-size threshold to invent here.

Held, the edge is gone the moment the follower has caught up or the leader stalls.
CategoryActiveReversal (the book has turned against the position) is the urgent stop;
CategoryAnchorStall (leadership exhausted, the move being chased is dead) and
CategorySynchronizedDrift (the follower now moves in lockstep — it has completed the
catch-up and is just systemic beta) harvest the position before the edge decays.
*/
func NewLeadLagPerspective() *strategy {
	return newStrategy(RegimeTrending, leadLagBranches)
}

var leadLagBranches = []Branch{
	entryBranch(CategoryInefficientLag,
		holdingExitBranch(
			CategoryActiveReversal,
			CategoryAnchorStall,
			CategorySynchronizedDrift,
		),
	),
}
