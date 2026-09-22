@0x81a3987fab9671e5;

using Go = import "/go.capnp";
$Go.package("graph");
$Go.import("github.com/theapemachine/symm/nomagique/graph");

interface ConnectedComponents {
  write @0 (
    fromNodes :List(Int64),
    toNodes :List(Int64),
  ) -> stream;

  done @1 () -> (
    componentCount :Int32,
    componentSizes :List(Int64),
  );
}
