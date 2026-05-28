package toxicity

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

const sym = "AAA/EUR"

var pair = asset.Pair{Wsname: sym, Quote: "EUR"}
var base = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// A large near-touch bid block that is cancelled (no covering trades) while
// other depth remains on that side must be flagged toxic.
func TestTrackerFlagsCancelledNearTouchBlockToxic(t *testing.T) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)
	tr.ApplyOrder(sym, pair, "add", "filler", 'b', 99.5, 50, base, base) // remaining depth
	tr.ApplyOrder(sym, pair, "add", "wall", 'b', 99.95, 10, base, base)  // the block

	// No trades near 99.95, so removing the wall is a cancel.
	tr.ApplyOrder(sym, pair, "delete", "wall", 'b', 99.95, 10, base, base)

	if !tr.IsToxic(sym, 99.95, base) {
		t.Fatal("expected a large young near-touch cancel to be flagged toxic")
	}
}

// The same block, but trades cover it: that is a fill, not a cancel, so it is
// never flagged toxic.
func TestTrackerFilledBlockIsNotToxic(t *testing.T) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)
	tr.ApplyOrder(sym, pair, "add", "filler", 'b', 99.5, 50, base, base)
	tr.ApplyOrder(sym, pair, "add", "wall", 'b', 99.95, 10, base, base)
	tr.ObserveTrade(sym, pair, 99.95, 10, base) // covers the wall

	tr.ApplyOrder(sym, pair, "delete", "wall", 'b', 99.95, 10, base, base)

	if tr.IsToxic(sym, 99.95, base) {
		t.Fatal("a filled block must not be flagged toxic")
	}
}

func TestTrackerToxicFlagExpires(t *testing.T) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)
	tr.ApplyOrder(sym, pair, "add", "filler", 'b', 99.5, 50, base, base)
	tr.ApplyOrder(sym, pair, "add", "wall", 'b', 99.95, 10, base, base)
	tr.ApplyOrder(sym, pair, "delete", "wall", 'b', 99.95, 10, base, base)

	if !tr.IsToxic(sym, 99.95, base) {
		t.Fatal("expected toxic immediately after the cancel")
	}

	if tr.IsToxic(sym, 99.95, base.Add(toxicCooldown+time.Second)) {
		t.Fatal("toxic flag must expire after the cooldown")
	}
}

// A small cancel (below largeBlockFrac of the remaining side depth) is not toxic.
func TestTrackerSmallCancelNotToxic(t *testing.T) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)
	tr.ApplyOrder(sym, pair, "add", "filler", 'b', 99.5, 50, base, base)
	tr.ApplyOrder(sym, pair, "add", "tiny", 'b', 99.95, 1, base, base)
	tr.ApplyOrder(sym, pair, "delete", "tiny", 'b', 99.95, 1, base, base)

	if tr.IsToxic(sym, 99.95, base) {
		t.Fatal("a small cancel (1 of remaining 50) must not be toxic")
	}
}

// A cancel far from mid is not toxic even if large and young.
func TestTrackerFarCancelNotToxic(t *testing.T) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)
	tr.ApplyOrder(sym, pair, "add", "filler", 'b', 88, 50, base, base)
	tr.ApplyOrder(sym, pair, "add", "deep", 'b', 90, 10, base, base) // 10% off mid
	tr.ApplyOrder(sym, pair, "delete", "deep", 'b', 90, 10, base, base)

	if tr.IsToxic(sym, 90, base) {
		t.Fatal("a cancel far from mid must not be flagged toxic")
	}
}

// An old cancel (older than toxicMaxAge) is not toxic: the block rested long
// enough to be genuine liquidity.
func TestTrackerOldCancelNotToxic(t *testing.T) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)
	tr.ApplyOrder(sym, pair, "add", "filler", 'b', 99.5, 50, base, base)
	tr.ApplyOrder(sym, pair, "add", "wall", 'b', 99.95, 10, base, base)

	old := base.Add(toxicMaxAge + time.Second)
	tr.ApplyOrder(sym, pair, "delete", "wall", 'b', 99.95, 10, base, old)

	if tr.IsToxic(sym, 99.95, old) {
		t.Fatal("a long-resting (old) cancel must not be flagged toxic")
	}
}

// Asks pulled (cancelled) while bids keep filling => intent to move price up =>
// Momentum / ask_pull.
func TestTrackerMeasureAskPull(t *testing.T) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)

	// Bid side fills.
	tr.ApplyOrder(sym, pair, "add", "b1", 'b', 99.95, 10, base, base)
	tr.ObserveTrade(sym, pair, 99.95, 10, base)
	tr.ApplyOrder(sym, pair, "delete", "b1", 'b', 99.95, 10, base, base)

	// Ask side is pulled (no covering trades).
	tr.ApplyOrder(sym, pair, "add", "a1", 'a', 100.05, 10, base, base)
	tr.ApplyOrder(sym, pair, "delete", "a1", 'a', 100.05, 10, base, base)

	m, ok := tr.Measure(sym, base)

	if !ok {
		t.Fatal("expected a bookflow measurement")
	}

	if m.Type != engine.Momentum || m.Source != "bookflow" || m.Reason != "ask_pull" {
		t.Fatalf("expected Momentum/bookflow/ask_pull, got %v/%q/%q", m.Type, m.Source, m.Reason)
	}

	if m.Confidence <= 0 || m.Confidence >= 1 {
		t.Fatalf("confidence must be in (0,1), got %v", m.Confidence)
	}
}

// Bids pulled while asks keep filling => Dump / bid_pull (mirror).
func TestTrackerMeasureBidPull(t *testing.T) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)

	tr.ApplyOrder(sym, pair, "add", "a1", 'a', 100.05, 10, base, base)
	tr.ObserveTrade(sym, pair, 100.05, 10, base)
	tr.ApplyOrder(sym, pair, "delete", "a1", 'a', 100.05, 10, base, base)

	tr.ApplyOrder(sym, pair, "add", "b1", 'b', 99.95, 10, base, base)
	tr.ApplyOrder(sym, pair, "delete", "b1", 'b', 99.95, 10, base, base)

	m, ok := tr.Measure(sym, base)

	if !ok || m.Type != engine.Dump || m.Reason != "bid_pull" {
		t.Fatalf("expected Dump/bid_pull, got ok=%v %v/%q", ok, m.Type, m.Reason)
	}
}

// An amend that cuts quantity at the same price is a partial removal and joins
// the trade tape like any other removal.
func TestTrackerAmendQuantityCutIsClassified(t *testing.T) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)
	tr.ApplyOrder(sym, pair, "add", "filler", 'b', 99.5, 50, base, base)
	tr.ApplyOrder(sym, pair, "add", "wall", 'b', 99.95, 12, base, base)
	// Cut 10 of the 12 with no covering trades: a cancel of 10.
	tr.ApplyOrder(sym, pair, "amend", "wall", 'b', 99.95, 2, base, base)

	if !tr.IsToxic(sym, 99.95, base) {
		t.Fatal("a large near-touch quantity cut with no trades should flag toxic")
	}
}

// L2 fallback: a per-level decrement with no covering trades classifies as a
// cancel and flags toxic, exactly like the L3 path.
func TestTrackerL2FallbackCancelToxic(t *testing.T) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)
	tr.ApplyBookLevel(sym, pair, 'b', 99.5, 50, base)  // remaining depth
	tr.ApplyBookLevel(sym, pair, 'b', 99.95, 10, base) // the block
	tr.ApplyBookLevel(sym, pair, 'b', 99.95, 0, base)  // level cleared (cancel)

	if !tr.IsToxic(sym, 99.95, base) {
		t.Fatal("L2 fallback: a cancelled near-touch level should flag toxic")
	}
}

// L2 fallback: a level reduced while trades cover the reduction is a fill.
func TestTrackerL2FallbackFillNotToxic(t *testing.T) {
	tr := NewTracker()
	tr.ObserveMid(sym, pair, 100)
	tr.ApplyBookLevel(sym, pair, 'b', 99.5, 50, base)
	tr.ApplyBookLevel(sym, pair, 'b', 99.95, 10, base)
	tr.ObserveTrade(sym, pair, 99.95, 10, base) // covers the reduction
	tr.ApplyBookLevel(sym, pair, 'b', 99.95, 0, base)

	if tr.IsToxic(sym, 99.95, base) {
		t.Fatal("L2 fallback: a filled level reduction must not be toxic")
	}
}

func TestTrackerUnknownSymbolIsSilent(t *testing.T) {
	tr := NewTracker()

	if tr.IsToxic("NOPE/EUR", 1, base) {
		t.Fatal("unknown symbol must not be toxic")
	}

	if _, ok := tr.Measure("NOPE/EUR", base); ok {
		t.Fatal("unknown symbol must not produce a measurement")
	}
}
