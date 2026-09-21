using Go = import "/go.capnp";
@0x9fc8a72b84fca830;
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

interface SellCount {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
