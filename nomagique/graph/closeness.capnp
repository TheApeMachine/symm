@0xc8c4d4f1c9de4394;

using Go = import "/go.capnp";
$Go.package("graph");
$Go.import("github.com/theapemachine/symm/nomagique/graph");

interface Closeness {
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
