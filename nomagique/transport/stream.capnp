using Go = import "/go.capnp";
@0x923022abf2007907;
$Go.package("transport");
$Go.import("nomagique/transport");

interface Stream {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
