@0xa16e23c455da8d34;

using Go = import "/go.capnp";
$Go.package("graph");
$Go.import("github.com/theapemachine/symm/nomagique/graph");

interface PageRank {
  write @0 (
    fromNodes :List(Int64),
    toNodes :List(Int64),
    damping :Float64,
    tol :Float64,
  ) -> stream;

  done @1 () -> (
    nodes :List(Int64),
    ranks :List(Float64),
  );
}
