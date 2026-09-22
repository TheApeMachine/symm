@0x9a8ea6187e482f91;

using Go = import "/go.capnp";
$Go.package("graph");
$Go.import("github.com/theapemachine/symm/nomagique/graph");

interface HITS {
  write @0 (
    fromNodes :List(Int64),
    toNodes :List(Int64),
    tol :Float64,
  ) -> stream;

  done @1 () -> (
    nodes :List(Int64),
    hubs :List(Float64),
    authorities :List(Float64),
  );
}
