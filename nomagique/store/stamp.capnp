using Go = import "/go.capnp";
@0xce39082bd76a65f1;
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

# Stamp is a publication clock, not a wall-clock bucket. An absent observation
# does not advance it. The original receipt remains in data unchanged.
interface Stamp {
  write @0 (data :Data, run :Text) -> stream;
  done @1 () -> Stamped;
}
struct Stamped {
  union {
    idle @0 :Void;
    item :group {
      data @1 :Data;
      run @2 :Text;
      sequence @3 :Int64;
    }
  }
}
