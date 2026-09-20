@0xdc4278cd52fcf274;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

struct KVPair {
  key @0 :Data;
  value @1 :Data;
}

interface Memory {
  get @0 (key :Data) -> stream;
  seekPrefix @1 (prefix :Data) -> stream;
  cas @2 (updates :List(KVPair)) -> stream;
  getStep @3 () -> stream;
  incrementStep @4 () -> stream;
  done @5 ();
}
