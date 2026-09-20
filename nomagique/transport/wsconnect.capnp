using Go = import "/go.capnp";
@0x9aff9f692e2a875e;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireWSConnect { url @0 :Text; }

interface WSConnect {
  write @0 (payload :WireWSConnect) -> stream;
  done @1 ();
}
