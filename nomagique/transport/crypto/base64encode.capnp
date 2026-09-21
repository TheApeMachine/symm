using Go = import "/go.capnp";
@0x9d1dc5facc5148a2;
$Go.package("crypto");
$Go.import("github.com/theapemachine/symm/nomagique/transport/crypto");

interface Base64Encode {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
