@0x84ce179ad0b00f0b;

using Go = import "/go.capnp";
$Go.package("graph");
$Go.import("github.com/theapemachine/symm/nomagique/graph");

interface MinimumSpanningTree {
  write @0 (
    fromNodes :List(Int64),
    toNodes :List(Int64),
    weights :List(Float64),
  ) -> stream;

  done @1 () -> (
    totalWeight :Float64,
    mstFrom :List(Int64),
    mstTo :List(Int64),
  );
}
