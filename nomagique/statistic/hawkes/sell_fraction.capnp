using Go = import "/go.capnp";
@0xcb4db620b20f3b42;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

struct WireSellFraction {
  payload @0 :AnyPointer;
}

interface SellFraction {
  write @0 (view :WireSellFraction) -> stream;
  done @1 ();
}
