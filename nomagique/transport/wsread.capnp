using Go = import "/go.capnp";
@0xde3a37ac2849ac5d;
$Go.package("transport");
$Go.import("nomagique/transport");

interface WSRead {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
