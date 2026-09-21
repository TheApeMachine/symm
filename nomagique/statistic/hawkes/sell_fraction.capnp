using Go = import "/go.capnp";
@0xa9e7c4f42ceec946;
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

interface SellFraction {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
