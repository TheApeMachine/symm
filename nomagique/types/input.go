package types

import "github.com/theapemachine/symm/nomagique"

/*
Input contracts are the shared numeric vocabulary that lets nomagique presets
compose. Under the hood every primitive reads and writes plain float64 slots in
a Frame; these interned symbols are the generic names multiple primitives can
assume, so the output of one preset (a Quantity, say) plugs directly into the
input of another without signal-specific renaming.

A producer that lifts raw market rows into this vocabulary only has to put the
right slots; a consumer only has to Get the same slots. Nothing else in the
numeric contract is signal-specific.
*/
var (
	// Quantity is the one primary observable flowing through a preset: executable
	// depth, Hawkes intensity, midpoint deviation, and similar single-valued
	// measures all occupy this slot. It is the same slot the engine already
	// treats as SampleValue, so presets and legacy primitives share one number.
	Quantity = nomagique.SampleValue

	// AlphaQuantity is a first secondary observable that joins Quantity in a
	// two-channel operation, typically the primary adaptation coefficient or the
	// buy/leading side of a pairwise measure.
	AlphaQuantity = MustIntern("input/alpha_quantity")

	// BetaQuantity is the second secondary observable alongside Quantity and
	// AlphaQuantity, typically the sell/lagging side of a pairwise measure.
	BetaQuantity = MustIntern("input/beta_quantity")

	// AlphaPrice and BetaPrice carry the two price observations paired with the
	// alpha and beta quantities, for two-touch instruments like an executable
	// depth measure. They stay numeric slots, so any two-price instrument can
	// occupy them.
	AlphaPrice = MustIntern("input/alpha_price")
	BetaPrice  = MustIntern("input/beta_price")

	// EventTimeSec and EventTimeNsec carry the normalized event clock every stateful
	// preset observes; they let windows and baselines adapt to the data's own
	// inter-arrival spacing instead of a fixed horizon. These are the same
	// interned slots the primitives already read as unix_sec/unix_nsec.
	EventTimeSec  = MustIntern("unix_sec")
	EventTimeNsec = MustIntern("unix_nsec")

	// Span is the control channel a baseline emits and a window consumes: the
	// window's target size in samples. It is a plain slot so any stability
	// estimator can drive any retention primitive through Configure.
	Span = MustIntern("input/span")
)
