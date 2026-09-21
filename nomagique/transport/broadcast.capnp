using Go = import "/go.capnp";
@0xab2c34cea45a1972;
$Go.package("transport");
$Go.import("nomagique/transport");

interface Broadcast {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
