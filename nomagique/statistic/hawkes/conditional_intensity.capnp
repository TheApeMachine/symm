using Go = import "/go.capnp";
@0xc367676e27bcfb9c;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

interface ConditionalIntensity {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
