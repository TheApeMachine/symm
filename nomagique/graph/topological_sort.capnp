@0xb4b0881ae8dc9ab5;

using Go = import "/go.capnp";
$Go.package("graph");
$Go.import("github.com/theapemachine/symm/nomagique/graph");

interface TopologicalSort {
  write @0 (
    fromNodes :List(Int64),
    toNodes :List(Int64),
  ) -> stream;

  done @1 () -> (
    order :List(Int64),
    hasCycle :Bool,
  );
}
