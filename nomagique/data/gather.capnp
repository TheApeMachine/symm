using Go = import "/go.capnp";
@0x840dc7107531e4e3;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Gather holds, as one list, the numbers its producers delivered on this
# evaluation: one slot per wired producer, in wiring order, with present
# saying which of them delivered. A slot whose producer did not deliver is
# unknown, never zero.
#
# When families are declared, it maintains retained readiness coverage until all
# declared signal families have contributed at least once, staying idle until
# the coordinate universe is ready.
struct Gathered {
  readiness @3 :Data;
  phase     @4 :Text;
  union {
    idle @0 :Void;
    gathered :group {
      values  @1 :List(Float64);
      present @2 :List(Bool);
    }
  }
}

interface Gather {
  write @0 (values :List(Float64), present :List(Bool), families :Text) -> stream;
  done @1 () -> Gathered;
}
