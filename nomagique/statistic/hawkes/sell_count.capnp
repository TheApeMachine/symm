using Go = import "/go.capnp";
@0x9fc8a72b84fca830;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

interface SellCount {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}
