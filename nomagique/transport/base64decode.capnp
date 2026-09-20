using Go = import "/go.capnp";
@0xcccd5dccee73e169;
$Go.package("transport");
$Go.import("nomagique/transport");

interface Base64Decode {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
