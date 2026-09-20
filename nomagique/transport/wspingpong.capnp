using Go = import "/go.capnp";
@0x91a1c67139ef6e9d;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireWSPingPong { connection @0 :AnyPointer; }

interface WSPingPong {
  write @0 (payload :WireWSPingPong) -> stream;
  done @1 ();
}
