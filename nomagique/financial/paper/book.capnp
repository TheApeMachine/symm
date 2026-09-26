@0xa1b07a61838e6a63;

using Go = import "/go.capnp";
$Go.package("paper");
$Go.import("github.com/theapemachine/symm/nomagique/financial/paper");

# Book reconciles level3 frames into one canonical order book per symbol.
# The venue checksum must agree; a missing or invalidated book stays absent
# until a valid snapshot arrives. Depth is the subscribed venue depth.
# One evaluation takes at most one frame; normalized data may be one object.
# Only external venue frames are JSON. Outputs are native Cap'n Proto fields.
using import "../../runtime/status.capnp".Standing;

interface Book extends(Standing) {
  write @0 (frame :List(Data), depth :Int64) -> stream;
  # Native metric projection. Slots are bid, ask, touch bid quantity, touch ask
  # quantity, total bid quantity, total ask quantity, observed bid notional,
  # observed ask notional, bid mutation count, ask mutation count.
  # Touch/depth require a reconciled book. Flow describes this message only;
  # delete mutations contribute a count but no displayed notional.
  # Trade-only slots 10..16 are bracket quantity, matched bid quantity,
  # matched ask quantity, bid match indicator, ask match indicator,
  # pre-trade bid touch quantity, pre-trade ask touch quantity. These require
  # the market's reconciled touch and never trigger the level3-only slots.
  done @1 () -> (values :List(Float64), present :List(Bool), reconciled :UInt64, market :Market);
}

# Immutable projection of the canonical reconciled book at this observation.
# updated distinguishes a new executable L3 book from a carried causal quote.
struct Market {
 symbol @0 :Text;
 bids @1 :List(Level);
 asks @2 :List(Level);
 updated @3 :Bool;
 # Reconciled per-order queue is emitted only for an updated L3 book.
 orders @4 :List(Order);
 struct Order { id @0 :Text; bid @1 :Bool; price @2 :Text; quantity @3 :Text; rank @4 :UInt32; }
 struct Level { price @0 :Text; quantity @1 :Text; }
}
