using Go = import "/go.capnp";
@0xf017a47b19b7d519;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

interface SellIntensity {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}
