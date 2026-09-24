using Go = import "/go.capnp";
@0x81c13a6a7de810da;
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

using import "../runtime/status.capnp".Standing;

# Hold keeps the latest reading of a series and reports it on every
# evaluation, so a series that arrives on its own evaluations can be read
# beside one that arrives on others: a futures quote against the spot price
# last seen, one instrument against the reference last seen.
#
# value gathers the producer's reading and present says whether it delivered
# one this evaluation; an evaluation without one leaves the held reading as it
# was. fresh says whether the held reading arrived on this evaluation. scope
# names the series; a new scope holds nothing until its first reading. Before
# any reading the hold is idle.
interface Hold extends(Standing) {
  write @0 (value :List(Float64), present :List(Bool), scope :Text) -> stream;
  done @1 () -> Held;
}

struct Held {
  union {
    idle @0 :Void;
    held :group {
      value @1 :Float64;
      fresh @2 :Bool;
    }
  }
}
