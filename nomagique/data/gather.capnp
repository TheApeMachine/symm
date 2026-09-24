using Go = import "/go.capnp";
@0x840dc7107531e4e3;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Gather holds, as one list, the numbers its producers delivered on this
# evaluation: one slot per wired producer, in wiring order, with present
# saying which of them delivered. A slot whose producer did not deliver is
# unknown, never zero.
#
# It hands out what arrived on the same evaluation, so a consumer reads a
# producer's number on the pass it was produced. An evaluation on which no
# producer delivered is idle, so nothing downstream runs on it.
struct Gathered {
  union {
    idle @0 :Void;
    gathered :group {
      values  @1 :List(Float64);
      present @2 :List(Bool);
    }
  }
}

interface Gather {
  write @0 (values :List(Float64), present :List(Bool)) -> stream;
  done @1 () -> Gathered;
}
