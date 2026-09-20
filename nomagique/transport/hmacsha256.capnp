using Go = import "/go.capnp";
@0x8de09eaaee89afbc;
$Go.package("transport");
$Go.import("nomagique/transport");

interface HMACSHA256 {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
