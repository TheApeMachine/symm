using Go = import "/go.capnp";
@0x8de09eaaee89afbc;
$Go.package("crypto");
$Go.import("github.com/theapemachine/symm/nomagique/transport/crypto");

# HMACSHA256 authenticates data under key. A missing key is an error, never
# an unkeyed digest.
interface HMACSHA256 {
  write @0 (data :Data, key :Data) -> stream;
  done @1 () -> (out :Data);
}
