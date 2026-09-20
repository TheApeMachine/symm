using Go = import "/go.capnp";
@0xce2530a4a850b491;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

struct WireConditionalIntensity {
  payload @0 :AnyPointer;
}

interface ConditionalIntensity {
  write @0 (view :WireConditionalIntensity) -> stream;
  done @1 ();
}
