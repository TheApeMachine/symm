using Go = import "/go.capnp";
@0xc6aad7fa07e8d55e;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

struct WireBuyCount {
  payload @0 :AnyPointer;
}

interface BuyCount {
  write @0 (view :WireBuyCount) -> stream;
  done @1 ();
}
