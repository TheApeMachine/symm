using Go = import "/go.capnp";
@0xc1b962b023871028;
$Go.package("crypto");
$Go.import("github.com/theapemachine/symm/nomagique/transport/crypto");

interface BearerAuth {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
