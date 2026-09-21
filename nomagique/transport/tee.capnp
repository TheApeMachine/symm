using Go = import "/go.capnp";
@0x85a37dbf10b9d53f;
$Go.package("transport");
$Go.import("github.com/theapemachine/symm/nomagique/transport");

interface Tee {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
