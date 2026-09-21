using Go = import "/go.capnp";
@0xa9aa4e1647796f92;
$Go.package("transport");
$Go.import("github.com/theapemachine/symm/nomagique/transport");

interface JSONEncode {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
