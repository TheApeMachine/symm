using Go = import "/go.capnp";
@0x92f602446f2501a3;
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

interface BuyCount {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
