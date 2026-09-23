using Go = import "/go.capnp";
@0xbc1534465ace77d2;
$Go.package("crypto");
$Go.import("github.com/theapemachine/symm/nomagique/transport/crypto");

# HMACSHA512 authenticates data under key. A missing key is an error, never
# an unkeyed digest.
interface HMACSHA512 {
  write @0 (data :Data, key :Data) -> stream;
  done @1 () -> (out :Data);
}
