using Go = import "/go.capnp";
@0xc43021d8a6551309;
$Go.package("transport");
$Go.import("nomagique/transport");

interface SHA256 {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
