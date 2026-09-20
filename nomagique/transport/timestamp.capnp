using Go = import "/go.capnp";
@0xea5633ada0f41888;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireTimestamp { }

interface Timestamp {
  write @0 (payload :WireTimestamp) -> stream;
  done @1 ();
}
