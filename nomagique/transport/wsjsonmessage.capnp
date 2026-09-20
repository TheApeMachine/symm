using Go = import "/go.capnp";
@0xb94a4aa5bbd82769;
$Go.package("transport");
$Go.import("nomagique/transport");

interface WSJSONMessage {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
