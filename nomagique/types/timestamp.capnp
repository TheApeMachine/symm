using Go = import "/go.capnp";
@0xea5633ada0f41888;
$Go.package("types");
$Go.import("nomagique/types");

interface Timestamp {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
