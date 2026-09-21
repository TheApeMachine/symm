using Go = import "/go.capnp";
@0xfb3636f33cf23058;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

interface BuyIntensity {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
