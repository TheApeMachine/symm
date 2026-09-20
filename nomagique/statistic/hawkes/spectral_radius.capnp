using Go = import "/go.capnp";
@0x8d5d02824600fad9;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

struct WireSpectralRadius {
  payload @0 :AnyPointer;
}

interface SpectralRadius {
  write @0 (view :WireSpectralRadius) -> stream;
  done @1 ();
}
