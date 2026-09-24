@0xa4cb9b0424f3133c;

using Go = import "/go.capnp";
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

# Trail keeps the most recent arrivals, as a JSON array oldest first: JSON
# values on values, numbers on numbers (only the slots present flags as
# delivered). capacity is how many it keeps, a declared display span: what a
# surface shows, never a horizon anything is computed over. Until something
# has arrived it is idle. scope names the series the arrivals belong to; it
# gathers, and a new scope starts the trail empty.
interface Trail {
  write @0 (
    values   :List(Data),
    numbers  :List(Float64),
    present  :List(Bool),
    capacity :UInt32,
    scope    :List(Text)
  ) -> stream;
  done @1 () -> Trailed;
}

struct Trailed {
  union {
    idle @0 :Void;
    out  @1 :Data;
  }
}
