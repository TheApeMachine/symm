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
interface Book {
  write @0 (frame :List(Data), depth :Int64) -> stream;
  done @1 () -> (out :Data);
}
