using Go = import "/go.capnp";
@0xab88c0378ea9b3b8;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

interface BuyRate {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}
