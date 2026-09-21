using Go = import "/go.capnp";
@0xbc1534465ace77d2;
$Go.package("crypto");
$Go.import("github.com/theapemachine/symm/nomagique/transport/crypto");

interface HMACSHA512 {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
