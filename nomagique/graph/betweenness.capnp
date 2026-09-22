@0xe2dee159a40da966;

using Go = import "/go.capnp";
$Go.package("graph");
$Go.import("github.com/theapemachine/symm/nomagique/graph");

interface Betweenness {
  write @0 (
    fromNodes :List(Int64),
    toNodes :List(Int64),
    weights :List(Float64),
  ) -> stream;

  done @1 () -> (
    nodes :List(Int64),
    scores :List(Float64),
  );
}
