using Go = import "/go.capnp";
@0xda7d76c759c3aa9e;
$Go.package("transport");
$Go.import("nomagique/transport");

interface Discard {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
