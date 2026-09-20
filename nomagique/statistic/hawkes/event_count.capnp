using Go = import "/go.capnp";
@0xbfed4393e2a1e113;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

struct WireEventCount {
  payload @0 :AnyPointer;
}

interface EventCount {
  write @0 (view :WireEventCount) -> stream;
  done @1 ();
}
