@0x85c56446d473810a;

using Go = import "/go.capnp";
$Go.package("graph");
$Go.import("github.com/theapemachine/symm/nomagique/graph");

interface ShortestPath {
  write @0 (
    fromNodes :List(Int64),
    toNodes :List(Int64),
    weights :List(Float64),
    source :Int64,
    target :Int64,
  ) -> stream;

  done @1 () -> (
    path :List(Int64),
    weight :Float64,
  );
}
