using Go = import "/go.capnp";
@0x9aff9f692e2a875e;
$Go.package("transport");
$Go.import("nomagique/transport");

interface WSConnect {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
