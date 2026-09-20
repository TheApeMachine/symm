using Go = import "/go.capnp";
@0xe833591d4ac0e10b;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

struct WireSellCount {
  payload @0 :AnyPointer;
}

interface SellCount {
  write @0 (view :WireSellCount) -> stream;
  done @1 ();
}
