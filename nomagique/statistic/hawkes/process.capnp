using Go = import "/go.capnp";
@0xb6ad448353121c47;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

struct WireProcess {
  payload @0 :AnyPointer;
}

interface Process {
  write @0 (process :WireProcess) -> stream;
  done @1 ();
}
