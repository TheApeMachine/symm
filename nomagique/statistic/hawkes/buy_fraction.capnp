using Go = import "/go.capnp";
@0xfa0e7d84976247ac;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

struct WireBuyFraction {
  payload @0 :AnyPointer;
}

interface BuyFraction {
  write @0 (view :WireBuyFraction) -> stream;
  done @1 ();
}
