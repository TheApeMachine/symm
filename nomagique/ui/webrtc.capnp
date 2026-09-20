using Go = import "/go.capnp";
@0x97d06ef4b12cbf41;
$Go.package("ui");
$Go.import("nomagique/ui");

struct WireWebRTCServer {
  payload @0 :AnyPointer;
  addr @1 :Text;
  path @2 :Text;
}

interface WebRTCServer {
  write @0 (server :WireWebRTCServer) -> stream;
  done @1 ();
}
