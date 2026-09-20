using Go = import "/go.capnp";
@0xded56e7128c04418;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

struct WireSellIntensity {
  payload @0 :AnyPointer;
}

interface SellIntensity {
  write @0 (view :WireSellIntensity) -> stream;
  done @1 ();
}
