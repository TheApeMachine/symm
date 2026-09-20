using Go = import "/go.capnp";
@0xb94a4aa5bbd82769;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireWSJSONMessage { type @0 :Text; payload @1 :AnyPointer; }

interface WSJSONMessage {
  write @0 (payload :WireWSJSONMessage) -> stream;
  done @1 ();
}
