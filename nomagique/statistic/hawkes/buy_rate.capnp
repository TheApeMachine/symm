using Go = import "/go.capnp";
@0xdfd7d977457b6c81;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

struct WireBuyRate {
  payload @0 :AnyPointer;
}

interface BuyRate {
  write @0 (view :WireBuyRate) -> stream;
  done @1 ();
}
