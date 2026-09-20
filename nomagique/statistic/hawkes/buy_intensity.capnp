using Go = import "/go.capnp";
@0x8efff55eefefb2ab;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

struct WireBuyIntensity {
  payload @0 :AnyPointer;
}

interface BuyIntensity {
  write @0 (view :WireBuyIntensity) -> stream;
  done @1 ();
}
