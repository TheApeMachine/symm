using Go = import "/go.capnp";
@0xede5e6e51d5440d7;
$Go.package("ui");
$Go.import("nomagique/ui");

struct WireBroadcast {
  payload @0 :AnyPointer;
  server @1 :AnyPointer;
}

interface Broadcast {
  write @0 (broadcast :WireBroadcast) -> stream;
  done @1 ();
}
