using Go = import "/go.capnp";
@0xa9aa4e1647796f92;
$Go.package("transport");
$Go.import("nomagique/transport");

interface JSONEncode {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
