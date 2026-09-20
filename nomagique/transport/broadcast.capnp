using Go = import "/go.capnp";
@0xab2c34cea45a1972;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireBroadcast { payload @0 :AnyPointer; channels @1 :List(Text); }

interface Broadcast {
  write @0 (payload :WireBroadcast) -> stream;
  done @1 ();
}
