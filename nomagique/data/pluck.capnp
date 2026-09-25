using Go = import "/go.capnp";
@0xad63d29b7f0ad0f7;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Pluck collects the value at path from each of the documents handed over
# together, in order, as one JSON array. A document without the path adds
# nothing. An evaluation that plucks nothing is idle.
interface Pluck {
  write @0 (data :List(Data), path :Text) -> stream;
  done @1 () -> Plucked;
}

struct Plucked {
  union {
    idle @0 :Void;
    out  @1 :Data;
  }
}
