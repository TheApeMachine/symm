using Go = import "/go.capnp";
@0x9d1dc5facc5148a2;
$Go.package("transport");
$Go.import("nomagique/transport");

interface Base64Encode {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
