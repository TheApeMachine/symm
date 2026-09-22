@0x84b152000a3544a0;

using Go = import "/go.capnp";
$Go.package("graph");
$Go.import("github.com/theapemachine/symm/nomagique/graph");

interface DirectedCycles {
  write @0 (
    fromNodes :List(Int64),
    toNodes :List(Int64),
  ) -> stream;

  done @1 () -> (
    cycleCount :Int32,
    hasCycles :Bool,
  );
}
