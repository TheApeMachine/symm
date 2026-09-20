using Go = import "/go.capnp";
@0xb9cf03445350777f;
$Go.package("transport");
$Go.import("nomagique/transport");

interface Route {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
