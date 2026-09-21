using Go = import "/go.capnp";
@0x9752b0b9c04840a6;
$Go.package("transport");
$Go.import("nomagique/transport");

interface WSClose {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
