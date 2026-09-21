using Go = import "/go.capnp";
@0xc165c886e177ee52;
$Go.package("transport");
$Go.import("github.com/theapemachine/symm/nomagique/transport");

interface Parallel {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
