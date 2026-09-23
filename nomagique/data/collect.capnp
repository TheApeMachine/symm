using Go = import "/go.capnp";
@0xe9ef64826c637195;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Collect appends JSON values in arrival order. A flush includes current inputs,
# emits a nonempty JSON array, then clears the collection. Empty flushes emit nothing.
interface Collect {
  write @0 (data :List(Data), flush :Bool) -> stream;
  done @1 () -> Collected;
}

struct Collected {
  union { idle @0 :Void; out @1 :Data; }
}
