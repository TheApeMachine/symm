@0xdc4278cd52fcf274;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

struct KVPair {
  key @0 :Data;
  value @1 :Data;
}

interface Memory {
  get @0 (key :Data) -> (value :Data);
  seekPrefix @1 (prefix :Data) -> (pairs :List(KVPair));
  cas @2 (updates :List(KVPair)) -> (ok :Bool);
  getStep @3 () -> (step :Int64);
  incrementStep @4 () -> (step :Int64);
  done @5 ();
}
