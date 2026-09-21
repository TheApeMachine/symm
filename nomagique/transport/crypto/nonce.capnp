using Go = import "/go.capnp";
@0xd0fb0fb33ce02b1c;
$Go.package("crypto");
$Go.import("github.com/theapemachine/symm/nomagique/transport/crypto");

interface Nonce {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
