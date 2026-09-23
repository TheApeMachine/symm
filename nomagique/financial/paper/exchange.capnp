@0x8ecad81a026b28d0;

using Go = import "/go.capnp";
$Go.package("paper");
$Go.import("github.com/theapemachine/symm/nomagique/financial/paper");

using import "../../store/radix.capnp".Retained;

# Exchange is one trading account at one exchange, replayed from the frames
# the exchange sent. It is where a decision meets what it would actually have
# made or lost.
#
# Frames rebuild each symbol's book order by order from Level 3 data and are
# verified against the exchange's checksum; a symbol without a snapshot, or
# whose checksum failed, has no book until its next snapshot. Instrument
# frames supply each pair's minimum quantity, minimum cost and increment.
# Depth is the subscribed Level 3 depth: the exchange does not delete orders
# that leave it, so the book is truncated to it.
#
# The account holds capital, in currency, and nothing else at the start. The
# balance is part of what is learned: ENTER while flat spends fraction of the
# cash not already committed, taker fee included; EXIT while holding sells the
# whole position; WAIT places nothing. Schedule is the exchange's asset pair
# table (entries carrying wsname, quote, cost_decimals and fees); the fee is
# the tier the account's own volume over the exchange's trailing fee window
# has reached, in tape time.
#
# Symbol and action are the decision, wired back from whatever read this
# exchange's state. They are committed after the evaluation, so an order
# rests until the next Level 3 frame for its symbol and fills against that
# book, never the one the decision saw. A market order walks the opposite
# side level by level; what the visible book cannot absorb stays unfilled.
# An order below the pair's minimums is refused whole, as the exchange would
# refuse it. An order whose symbol has no instrument rules resolves unknown.
#
# A round trip closes when its position is fully sold: its PnL is what that
# entry and that exit together made or lost, fees included. Each evaluation
# reports the oldest outcome not yet reported.
interface Exchange extends(Retained) {
  write @0 (
    frame    :Data,
    time     :Text,
    depth    :Int64,
    capital  :Text,
    currency :Text,
    fraction :Text,
    schedule :Data,
    symbol   :Text,
    action   :Text
  ) -> stream;
  done @1 () -> Account;
}

struct Account {
  union {
    quiet @0 :Void;
    closed :group {
      symbol   @1 :Text;
      opened   @2 :Text;
      closedAt @3 :Text;
      basis    @4 :Text;
      proceeds @5 :Text;
      pnl      @6 :Text;
      return   @7 :Float64;
    }
    refused :group {
      refusedSymbol @8 :Text;
      action        @9 :Text;
      reason        @10 :Text;
    }
  }
  cash      @11 :Text;
  available @12 :Text;
  holding   @13 :List(Text);
  volume    @14 :Text;
  resting   @15 :UInt64;
  books     @16 :UInt64;
}
