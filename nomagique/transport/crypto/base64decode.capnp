using Go = import "/go.capnp";
@0xcccd5dccee73e169;
$Go.package("crypto");
$Go.import("github.com/theapemachine/symm/nomagique/transport/crypto");

interface Base64Decode {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
