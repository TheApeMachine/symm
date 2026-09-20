using Go = import "/go.capnp";
@0xdf0315c46f0719ed;
$Go.package("ui");
$Go.import("nomagique/ui");

interface HTTPServer {
  write @0 (in :Data, addr :Text, path :Text) -> stream;
  done @1 () -> (out :Data);
}
