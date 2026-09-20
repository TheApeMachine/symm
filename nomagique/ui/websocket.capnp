using Go = import "/go.capnp";
@0x921515fc32b6ed0d;
$Go.package("ui");
$Go.import("nomagique/ui");

interface WebSocketServer {
  write @0 (in :Data, addr :Text, path :Text) -> stream;
  done @1 () -> (out :Data);
}
