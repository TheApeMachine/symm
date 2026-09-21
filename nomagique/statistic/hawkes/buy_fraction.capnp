using Go = import "/go.capnp";
@0xf221a7fe927e1f48;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

interface BuyFraction {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
