@0xa1b07a61838e6a63;

using Go = import "/go.capnp";
$Go.package("paper");
$Go.import("github.com/theapemachine/symm/nomagique/financial/paper");

# Book replays an exchange's frames into the market each symbol had.
#
# Level 3 frames rebuild the symbol's book order by order and must reconcile
# with the exchange's checksum; a symbol without a snapshot, or whose checksum
# failed, has no book until its next snapshot. Depth is the subscribed depth:
# the exchange does not delete orders that leave it, so the book is truncated
# to it. Instrument frames are kept per symbol as the exchange sent them.
#
# Frames gather: an evaluation without one reports {"symbol": ""}, and one
# evaluation takes at most one frame. A record may carry its data as one object
# rather than an array of one.
#
# out is the market the frame moved: {"symbol", "bids", "asks", "pair"}, with
# levels best first as [price, quantity] decimal strings and pair the symbol's
# latest instrument record. A frame that moved no reconciled book reports
# {"symbol": ""}, so every evaluation says what it knows.
using import "../../runtime/status.capnp".Standing;

interface Book extends(Standing) {
  write @0 (frame :List(Data), depth :Int64, encode :Bool = true) -> stream;
  # Native metric projection. Slots are bid, ask, touch bid quantity, touch ask
  # quantity, total bid quantity, total ask quantity, observed bid notional,
  # observed ask notional, bid mutation count, ask mutation count.
  # Touch/depth require a reconciled book. Flow describes this message only;
  # delete mutations contribute a count but no displayed notional.
  # Trade-only slots 10..16 are bracket quantity, matched bid quantity,
  # matched ask quantity, bid match indicator, ask match indicator,
  # pre-trade bid touch quantity, pre-trade ask touch quantity. These require
  # the market's reconciled touch and never trigger the level3-only slots.
  done @1 () -> (out :Data, values :List(Float64), present :List(Bool), reconciled :UInt64);
}
