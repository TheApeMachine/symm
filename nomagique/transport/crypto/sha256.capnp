using Go = import "/go.capnp";
@0xc43021d8a6551309;
$Go.package("crypto");
$Go.import("github.com/theapemachine/symm/nomagique/transport/crypto");

interface SHA256 {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
