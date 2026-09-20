using Go = import "/go.capnp";
@0xb93a7eb06bcecc4f;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

interface SpectralRadius {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}
