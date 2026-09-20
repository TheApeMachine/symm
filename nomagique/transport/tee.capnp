using Go = import "/go.capnp";
@0x85a37dbf10b9d53f;
$Go.package("transport");
$Go.import("nomagique/transport");

interface Tee {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
