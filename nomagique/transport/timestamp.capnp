using Go = import "/go.capnp";
@0xea5633ada0f41888;
$Go.package("transport");
$Go.import("nomagique/transport");

interface Timestamp {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
