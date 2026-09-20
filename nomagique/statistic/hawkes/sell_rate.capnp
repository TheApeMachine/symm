using Go = import "/go.capnp";
@0xa524854c667f3f19;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

struct WireSellRate {
  payload @0 :AnyPointer;
}

interface SellRate {
  write @0 (view :WireSellRate) -> stream;
  done @1 ();
}
