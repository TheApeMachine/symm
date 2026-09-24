using Go = import "/go.capnp";
@0xaa1fabd35fd803ca;
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

# Change is how far each element of a list moved since its previous reading.
#
# value and present are this reading; previous and known are the last reading
# of each element. An element moved only where it is present now and was
# known before; everywhere else its change is undefined, never zero. A change
# of exactly zero is an element that was read and did not move.
#
# read.index and read.latest are the elements read now and their values, which
# is what a store of previous readings is written with. When no reading
# arrived at all the change is idle: there is nothing to write back.
interface Change {
  write @0 (
    value    :List(Float64),
    present  :List(Bool),
    previous :List(Float64),
    known    :List(Bool)
  ) -> stream;
  done @1 () -> Changed;
}

struct Changed {
  change  @0 :List(Float64);
  defined @1 :List(Bool);

  union {
    idle @2 :Void;
    read :group {
      index  @3 :List(Int64);
      latest @4 :List(Float64);
    }
  }
}
