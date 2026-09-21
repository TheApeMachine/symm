using Go = import "/go.capnp";
@0x8de09eaaee89afbc;
$Go.package("crypto");
$Go.import("github.com/theapemachine/symm/nomagique/transport/crypto");

interface HMACSHA256 {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
