using Go = import "/go.capnp";
@0xc165c886e177ee52;
$Go.package("transport");
$Go.import("nomagique/transport");

interface Parallel {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
