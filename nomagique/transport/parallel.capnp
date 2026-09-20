using Go = import "/go.capnp";
@0xc165c886e177ee52;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireParallel { payloads @0 :AnyPointer; task @1 :AnyPointer; }

interface Parallel {
  write @0 (payload :WireParallel) -> stream;
  done @1 ();
}
