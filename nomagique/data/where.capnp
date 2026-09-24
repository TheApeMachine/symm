using Go = import "/go.capnp";
@0x9a02fb881780c11c;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Where keeps, of the documents handed over together, the ones whose value at
# path is the JSON value given, in the order they came. A document without the
# path is not kept: an absent value equals nothing. An evaluation that keeps
# nothing is idle.
interface Where {
  write @0 (data :List(Data), path :Text, value :Data) -> stream;
  done @1 () -> Kept;
}

struct Kept {
  union {
    idle @0 :Void;
    out  @1 :List(Data);
  }
}
