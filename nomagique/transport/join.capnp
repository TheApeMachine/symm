using Go = import "/go.capnp";
@0xf790ed006f8391af;
$Go.package("transport");
$Go.import("github.com/theapemachine/symm/nomagique/transport");

interface Join {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
