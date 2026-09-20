using Go = import "/go.capnp";
@0xa4d3b6bbf32f00be;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

struct WireAssemble {
  payload @0 :AnyPointer;
}

interface Assemble {
  write @0 (assemble :WireAssemble) -> stream;
  done @1 ();
}
