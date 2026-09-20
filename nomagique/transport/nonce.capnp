using Go = import "/go.capnp";
@0xd0fb0fb33ce02b1c;
$Go.package("transport");
$Go.import("nomagique/transport");

interface Nonce {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
