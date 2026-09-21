using Go = import "/go.capnp";
@0xfb9dbf8960eb25d6;
$Go.package("transport");
$Go.import("nomagique/transport");

interface WSBatch {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
