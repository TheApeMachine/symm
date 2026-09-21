using Go = import "/go.capnp";
@0xda7d76c759c3aa9e;
$Go.package("transport");
$Go.import("github.com/theapemachine/symm/nomagique/transport");

interface Discard {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
