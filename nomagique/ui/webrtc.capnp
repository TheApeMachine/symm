using Go = import "/go.capnp";
@0xc107316fcb5b03f0;
$Go.package("ui");
$Go.import("nomagique/ui");

interface WebRTCServer {
  write @0 (in :Data, addr :Text) -> stream;
  done @1 () -> (out :Data);
}
