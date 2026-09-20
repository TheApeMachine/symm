using Go = import "/go.capnp";
@0x921515fc32b6ed0d;
$Go.package("ui");
$Go.import("nomagique/ui");

struct WireWebSocketServer {
  payload @0 :AnyPointer;
  addr @1 :Text;
  path @2 :Text;
}

interface WebSocketServer {
  write @0 (server :WireWebSocketServer) -> stream;
  done @1 ();
}
