using Go = import "/go.capnp";
@0xa9e7c4f42ceec946;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

interface SellFraction {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}
