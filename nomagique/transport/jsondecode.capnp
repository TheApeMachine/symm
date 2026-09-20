using Go = import "/go.capnp";
@0x8060fbf5b51245a1;
$Go.package("transport");
$Go.import("nomagique/transport");

interface JSONDecode {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
