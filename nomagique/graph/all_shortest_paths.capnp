@0xbc38e2499b6e7449;

using Go = import "/go.capnp";
$Go.package("graph");
$Go.import("github.com/theapemachine/symm/nomagique/graph");

interface AllShortestPaths {
  write @0 (
    fromNodes :List(Int64),
    toNodes :List(Int64),
    weights :List(Float64),
  ) -> stream;

  done @1 () -> (
    distances :List(Float64),
    nodeCount :Int32,
  );
}
