using Go = import "/go.capnp";
@0xa1caaeef47f4fc66;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

interface SellRate {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}
