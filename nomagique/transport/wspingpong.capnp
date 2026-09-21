using Go = import "/go.capnp";
@0x91a1c67139ef6e9d;
$Go.package("transport");
$Go.import("nomagique/transport");

interface WSPingPong {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
