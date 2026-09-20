using Go = import "/go.capnp";
@0xaf8e3ce634bf9266;
$Go.package("transport");
$Go.import("nomagique/transport");

interface WSDecodeJSON {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
