using Go = import "/go.capnp";
@0xeec260166dcfeaf8;
$Go.package("crypto");
$Go.import("github.com/theapemachine/symm/nomagique/transport/crypto");

interface HeaderAuth {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
